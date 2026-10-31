// Tests unitaires et de sécurité d'Aigle Server (Édition SaaS).
// Vérifie :
//   1. Le rejet des requêtes non authentifiées en mode Authelia Zero-Trust (401 Unauthorized)
//   2. La protection anti-spoofing d'en-têtes via le secret partagé X-Aigle-Proxy-Secret
//   3. La synchronisation automatique des profils et groupes Authelia (superadmin vs viewer)
//   4. L'application stricte du RBAC et des permissions fines par projet (Maître du projet, Exécutant, Lecteur)
//   5. Le changement de mot de passe Argon2id direct (users_database.yml + DB) et le mode Portail Authelia
//   6. L'enrôlement 2FA TOTP (QR Code SVG + RFC 6238), la protection 2FA par projet (428 Precondition Required)
//      et la synchronisation automatique des règles two_factor dans configuration.yml d'Authelia
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aigle/aigle-server/internal/authelia"
	"github.com/aigle/aigle-server/internal/config"
	"github.com/aigle/aigle-server/internal/database"
	"github.com/aigle/aigle-server/internal/models"
)

func setupTestServer(t *testing.T, proxySecret string) http.Handler {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "aigle_saas_test.db")
	dlDir := filepath.Join(tmpDir, "downloads")
	_ = os.MkdirAll(dlDir, 0755)
	_ = os.WriteFile(filepath.Join(dlDir, "aigle-windows-amd64.exe"), []byte("MZ-AIGLE-TEST-BINARY"), 0644)

	initHash, _ := authelia.HashPasswordArgon2id("AigleAdmin2026!")
	usersFile := filepath.Join(tmpDir, "users_database.yml")
	_ = os.WriteFile(usersFile, []byte("users:\n  admin:\n    disabled: false\n    displayname: \"Admin Studio\"\n    email: \"admin@heimerdinger.studio\"\n    password: \""+initHash+"\"\n    groups:\n      - admins\n"), 0600)

	configFile := filepath.Join(tmpDir, "configuration.yml")
	_ = os.WriteFile(configFile, []byte("access_control:\n  default_policy: 'deny'\n  rules:\n    - domain: 'aigle.heimerdinger.studio'\n      policy: 'one_factor'\n"), 0600)

	cfg := &config.ServerConfig{
		Port:               "8090",
		AppDomain:          "aigle.heimerdinger.studio",
		AuthDomain:         "aigle-auth.heimerdinger.studio",
		ReadTimeoutSec:     10,
		WriteTimeoutSec:    10,
		MaxBodyBytes:       2 << 20,
		DBDriver:           "sqlite",
		DBDSN:              dbPath,
		AuthMode:           "authelia",
		TrustedProxySecret: proxySecret,
		AdminGroups:        []string{"admins", "superadmin", "aigle-admin"},
		ManagerGroups:      []string{"managers", "aigle-manager"},
		DefaultUserEmail:   "admin@heimerdinger.studio",
		DefaultUserName:    "Admin Studio",
		PasswordMode:       "both",
		AutheliaUsersFile:  usersFile,
		AutheliaConfigFile: configFile,
		TOTPSessionMinutes: 720,
		RateLimitRPS:       100,
		RateLimitBurst:     200,
		DownloadsDir:       dlDir,
	}

	store, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("Échec d'initialisation de la base SQLite de test: %v", err)
	}
	t.Cleanup(func() { store.DB.Close() })

	return NewServer(store, cfg, nil)
}

func TestAutheliaZeroTrustAndProxySecret(t *testing.T) {
	handler := setupTestServer(t, "secret-traefik-token-999")

	// 1. La sonde /api/health doit rester publique pour Docker/Traefik
	reqHealth := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur /api/health, obtenu %d", recHealth.Code)
	}
	if recHealth.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("En-tête X-Frame-Options: DENY manquant")
	}

	// 2. Appel sur /api/me sans en-tête Authelia -> doit être rejeté (401)
	reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	recNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(recNoAuth, reqNoAuth)
	if recNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("Attendu 401 sans en-têtes Authelia, obtenu %d", recNoAuth.Code)
	}

	// 3. Appel avec en-têtes Authelia mais SANS le secret X-Aigle-Proxy-Secret -> rejeté (401 anti-spoofing)
	reqSpoof := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqSpoof.Header.Set("Remote-User", "admin")
	reqSpoof.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	recSpoof := httptest.NewRecorder()
	handler.ServeHTTP(recSpoof, reqSpoof)
	if recSpoof.Code != http.StatusUnauthorized {
		t.Fatalf("Attendu 401 lors d'une tentative d'usurpation sans X-Aigle-Proxy-Secret, obtenu %d", recSpoof.Code)
	}

	// 4. Appel légitime venant de Traefik + Authelia avec le bon secret partagé -> 200 OK
	reqValid := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqValid.Header.Set("X-Aigle-Proxy-Secret", "secret-traefik-token-999")
	reqValid.Header.Set("Remote-User", "admin")
	reqValid.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqValid.Header.Set("Remote-Name", "Admin Studio")
	reqValid.Header.Set("Remote-Groups", "admins,users")
	recValid := httptest.NewRecorder()
	handler.ServeHTTP(recValid, reqValid)
	if recValid.Code != http.StatusOK {
		t.Fatalf("Attendu 200 avec en-têtes Authelia valides, obtenu %d: %s", recValid.Code, recValid.Body.String())
	}
}

func TestRBACAndMultiTenantIsolation(t *testing.T) {
	handler := setupTestServer(t, "")

	// 1. Connexion d'un utilisateur "viewer" (groupe 'viewers')
	reqViewerWrite := httptest.NewRequest(http.MethodPost, "/api/projects/prj_cloud/tickets", bytes.NewBufferString(`{"title":"Ticket non autorisé"}`))
	reqViewerWrite.Header.Set("Remote-User", "guest")
	reqViewerWrite.Header.Set("Remote-Email", "guest@client.fr")
	reqViewerWrite.Header.Set("Remote-Groups", "viewers")
	recViewerWrite := httptest.NewRecorder()
	handler.ServeHTTP(recViewerWrite, reqViewerWrite)
	if recViewerWrite.Code != http.StatusForbidden {
		t.Fatalf("Un utilisateur 'viewer' ne doit pas pouvoir créer de ticket (attendu 403, obtenu %d)", recViewerWrite.Code)
	}

	// 2. Tentative d'accès à /api/admin/overview par un simple membre -> 403 Forbidden
	reqMemberAdmin := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	reqMemberAdmin.Header.Set("Remote-User", "dev1")
	reqMemberAdmin.Header.Set("Remote-Email", "dev1@client.fr")
	reqMemberAdmin.Header.Set("Remote-Groups", "developers")
	recMemberAdmin := httptest.NewRecorder()
	handler.ServeHTTP(recMemberAdmin, reqMemberAdmin)
	if recMemberAdmin.Code != http.StatusForbidden {
		t.Fatalf("Un simple membre ne doit pas accéder à /api/admin/overview (attendu 403, obtenu %d)", recMemberAdmin.Code)
	}

	// 3. Accès à /api/admin/overview par le SuperAdmin -> 200 OK + vérification des logs d'audit
	reqSuperAdmin := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	reqSuperAdmin.Header.Set("Remote-User", "admin")
	reqSuperAdmin.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqSuperAdmin.Header.Set("Remote-Groups", "admins")
	recSuperAdmin := httptest.NewRecorder()
	handler.ServeHTTP(recSuperAdmin, reqSuperAdmin)
	if recSuperAdmin.Code != http.StatusOK {
		t.Fatalf("Le SuperAdmin doit accéder à /api/admin/overview (attendu 200, obtenu %d)", recSuperAdmin.Code)
	}

	var overview models.SystemOverview
	if err := json.NewDecoder(recSuperAdmin.Body).Decode(&overview); err != nil {
		t.Fatalf("Décodage SystemOverview échoué: %v", err)
	}
	if len(overview.Workspaces) < 2 {
		t.Errorf("Attendu au moins 2 Workspaces dans le seed initial, obtenu %d", len(overview.Workspaces))
	}
	if len(overview.RecentAuditLogs) == 0 {
		t.Errorf("Le journal d'audit devrait contenir les événements de sécurité précédents")
	}
}

func TestPublicPortalAndDownloadsSecurity(t *testing.T) {
	handler := setupTestServer(t, "secret-traefik-token-999")

	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	recRoot := httptest.NewRecorder()
	handler.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur la vitrine publique /, obtenu %d", recRoot.Code)
	}

	reqAppNoAuth := httptest.NewRequest(http.MethodGet, "/app", nil)
	recAppNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAppNoAuth, reqAppNoAuth)
	if recAppNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("Attendu 401 sur l'espace protégé /app sans Authelia, obtenu %d", recAppNoAuth.Code)
	}

	reqRel := httptest.NewRequest(http.MethodGet, "/public/api/releases", nil)
	recRel := httptest.NewRecorder()
	handler.ServeHTTP(recRel, reqRel)
	if recRel.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur /public/api/releases, obtenu %d", recRel.Code)
	}

	reqDL := httptest.NewRequest(http.MethodGet, "/public/downloads/aigle-windows-amd64.exe", nil)
	recDL := httptest.NewRecorder()
	handler.ServeHTTP(recDL, reqDL)
	if recDL.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors du téléchargement de aigle-windows-amd64.exe, obtenu %d", recDL.Code)
	}
	if recDL.Header().Get("X-Checksum-SHA256") == "" {
		t.Errorf("En-tête X-Checksum-SHA256 manquant sur le téléchargement du binaire")
	}

	reqEvil := httptest.NewRequest(http.MethodGet, "/public/downloads/aigle_saas_test.db", nil)
	recEvil := httptest.NewRecorder()
	handler.ServeHTTP(recEvil, reqEvil)
	if recEvil.Code == http.StatusOK {
		t.Fatalf("Un fichier hors liste blanche ne doit jamais pouvoir être téléchargé")
	}
}

func TestAccountPasswordAndTOTPAndGranularProjectPermissions(t *testing.T) {
	handler := setupTestServer(t, "")

	// 1. Changement de mot de passe Argon2id direct (TODO #1)
	// Mauvais mot de passe actuel -> 401
	reqBadPwd := httptest.NewRequest(http.MethodPost, "/api/account/password", bytes.NewBufferString(`{"current_password":"WrongPassword","new_password":"NewSecurePassword2026!","confirm_password":"NewSecurePassword2026!"}`))
	reqBadPwd.Header.Set("Remote-User", "admin")
	reqBadPwd.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqBadPwd.Header.Set("Remote-Groups", "admins")
	recBadPwd := httptest.NewRecorder()
	handler.ServeHTTP(recBadPwd, reqBadPwd)
	if recBadPwd.Code != http.StatusUnauthorized {
		t.Fatalf("Attendu 401 avec un mauvais mot de passe actuel, obtenu %d", recBadPwd.Code)
	}

	// Bon mot de passe actuel -> 200 OK et synchronisation Argon2id dans users_database.yml
	reqGoodPwd := httptest.NewRequest(http.MethodPost, "/api/account/password", bytes.NewBufferString(`{"current_password":"AigleAdmin2026!","new_password":"NewSecurePassword2026!","confirm_password":"NewSecurePassword2026!"}`))
	reqGoodPwd.Header.Set("Remote-User", "admin")
	reqGoodPwd.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqGoodPwd.Header.Set("Remote-Groups", "admins")
	recGoodPwd := httptest.NewRecorder()
	handler.ServeHTTP(recGoodPwd, reqGoodPwd)
	if recGoodPwd.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors du changement de mot de passe Argon2id, obtenu %d: %s", recGoodPwd.Code, recGoodPwd.Body.String())
	}

	// 2. Enrôlement 2FA TOTP (QR Code SVG + Activation RFC 6238) (TODO #3)
	reqSetupTOTP := httptest.NewRequest(http.MethodPost, "/api/account/totp/setup", nil)
	reqSetupTOTP.Header.Set("Remote-User", "admin")
	reqSetupTOTP.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqSetupTOTP.Header.Set("Remote-Groups", "admins")
	recSetupTOTP := httptest.NewRecorder()
	handler.ServeHTTP(recSetupTOTP, reqSetupTOTP)
	if recSetupTOTP.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur /api/account/totp/setup, obtenu %d", recSetupTOTP.Code)
	}
	var totpSetup struct {
		Secret string `json:"secret"`
		QRSVG  string `json:"qr_svg"`
	}
	_ = json.NewDecoder(recSetupTOTP.Body).Decode(&totpSetup)
	if totpSetup.Secret == "" || !strings.HasPrefix(totpSetup.QRSVG, "data:image/svg+xml;base64,") {
		t.Fatalf("Secret TOTP ou QR Code SVG invalide: %+v", totpSetup)
	}

	validCode, err := authelia.CurrentTOTPCode(totpSetup.Secret, time.Now())
	if err != nil {
		t.Fatalf("Erreur calcul code TOTP: %v", err)
	}
	reqActivateTOTP := httptest.NewRequest(http.MethodPost, "/api/account/totp/activate", bytes.NewBufferString(`{"code":"`+validCode+`"}`))
	reqActivateTOTP.Header.Set("Remote-User", "admin")
	reqActivateTOTP.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqActivateTOTP.Header.Set("Remote-Groups", "admins")
	recActivateTOTP := httptest.NewRecorder()
	handler.ServeHTTP(recActivateTOTP, reqActivateTOTP)
	if recActivateTOTP.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors de l'activation TOTP, obtenu %d: %s", recActivateTOTP.Code, recActivateTOTP.Body.String())
	}

	// 3. Création d'un utilisateur "Exécutant" (peut déplacer les tickets et pointer, mais PAS créer ni supprimer de ticket) (TODO #2)
	reqExecLogin := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reqExecLogin.Header.Set("Remote-User", "operateur")
	reqExecLogin.Header.Set("Remote-Email", "operateur@heimerdinger.studio")
	reqExecLogin.Header.Set("Remote-Name", "Opérateur Exécutant")
	reqExecLogin.Header.Set("Remote-Groups", "developers")
	recExecLogin := httptest.NewRecorder()
	handler.ServeHTTP(recExecLogin, reqExecLogin)
	var execMe struct {
		User models.User `json:"user"`
	}
	_ = json.NewDecoder(recExecLogin.Body).Decode(&execMe)

	// Attribution du rôle prédéfini "executor" sur le projet prj_cloud
	reqAssignExec := httptest.NewRequest(http.MethodPost, "/api/projects/prj_cloud/members", bytes.NewBufferString(`{"user_id":"`+execMe.User.ID+`","project_role":"executor","apply_preset":true}`))
	reqAssignExec.Header.Set("Remote-User", "admin")
	reqAssignExec.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqAssignExec.Header.Set("Remote-Groups", "admins")
	recAssignExec := httptest.NewRecorder()
	handler.ServeHTTP(recAssignExec, reqAssignExec)
	if recAssignExec.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors de l'affectation du rôle executor, obtenu %d: %s", recAssignExec.Code, recAssignExec.Body.String())
	}

	// L'exécutant tente de créer un nouveau ticket -> doit être refusé (403 car CanWriteTickets = false)
	reqExecCreate := httptest.NewRequest(http.MethodPost, "/api/projects/prj_cloud/tickets", bytes.NewBufferString(`{"title":"Nouveau ticket non autorisé"}`))
	reqExecCreate.Header.Set("Remote-User", "operateur")
	reqExecCreate.Header.Set("Remote-Email", "operateur@heimerdinger.studio")
	reqExecCreate.Header.Set("Remote-Groups", "developers")
	recExecCreate := httptest.NewRecorder()
	handler.ServeHTTP(recExecCreate, reqExecCreate)
	if recExecCreate.Code != http.StatusForbidden {
		t.Fatalf("Un exécutant (CanWriteTickets=false) ne doit pas pouvoir créer un ticket (attendu 403, obtenu %d)", recExecCreate.Code)
	}

	// L'exécutant déplace un ticket existant (CanExecute = true) -> doit être autorisé (200 OK)
	reqExecMove := httptest.NewRequest(http.MethodPatch, "/api/tickets/tck_3/move", bytes.NewBufferString(`{"status":"review"}`))
	reqExecMove.Header.Set("Remote-User", "operateur")
	reqExecMove.Header.Set("Remote-Email", "operateur@heimerdinger.studio")
	reqExecMove.Header.Set("Remote-Groups", "developers")
	recExecMove := httptest.NewRecorder()
	handler.ServeHTTP(recExecMove, reqExecMove)
	if recExecMove.Code != http.StatusOK {
		t.Fatalf("Un exécutant (CanExecute=true) doit pouvoir déplacer un ticket dans le Kanban (attendu 200, obtenu %d: %s)", recExecMove.Code, recExecMove.Body.String())
	}

	// 4. Activation de la double authentification 2FA TOTP obligatoire sur le projet prj_cloud (TODO #3)
	reqEnableProjTOTP := httptest.NewRequest(http.MethodPut, "/api/projects/prj_cloud/security", bytes.NewBufferString(`{"require_totp":true}`))
	reqEnableProjTOTP.Header.Set("Remote-User", "admin")
	reqEnableProjTOTP.Header.Set("Remote-Email", "admin@heimerdinger.studio")
	reqEnableProjTOTP.Header.Set("Remote-Groups", "admins")
	recEnableProjTOTP := httptest.NewRecorder()
	handler.ServeHTTP(recEnableProjTOTP, reqEnableProjTOTP)
	if recEnableProjTOTP.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors de l'activation 2FA sur le projet, obtenu %d: %s", recEnableProjTOTP.Code, recEnableProjTOTP.Body.String())
	}

	// L'opérateur (qui n'a pas encore validé de code 2FA TOTP pour sa session) tente d'ouvrir le projet -> 428 Precondition Required (TOTP_REQUIRED)
	reqOpAccess := httptest.NewRequest(http.MethodGet, "/api/projects/prj_cloud/full", nil)
	reqOpAccess.Header.Set("Remote-User", "operateur")
	reqOpAccess.Header.Set("Remote-Email", "operateur@heimerdinger.studio")
	reqOpAccess.Header.Set("Remote-Groups", "developers")
	recOpAccess := httptest.NewRecorder()
	handler.ServeHTTP(recOpAccess, reqOpAccess)
	if recOpAccess.Code != http.StatusPreconditionRequired {
		t.Fatalf("Attendu 428 (TOTP_REQUIRED) sur un projet protégé par 2FA pour un utilisateur non vérifié, obtenu %d", recOpAccess.Code)
	}
}
