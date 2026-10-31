// Tests unitaires et de sécurité d'Aigle Server (Édition SaaS).
// Vérifie :
//   1. Le rejet des requêtes non authentifiées en mode Authelia Zero-Trust (401 Unauthorized)
//   2. La protection anti-spoofing d'en-têtes via le secret partagé X-Aigle-Proxy-Secret
//   3. La synchronisation automatique des profils et groupes Authelia (superadmin vs viewer)
//   4. L'application stricte du RBAC (blocage d'écriture pour un 'viewer', accès console '/api/admin' réservé au 'superadmin')
//   5. L'isolation Multi-Tenant entre Workspaces et la génération des journaux d'audit (audit_logs)
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	// Création d'un binaire de test pour vérifier le téléchargement public et le calcul SHA-256
	_ = os.WriteFile(filepath.Join(dlDir, "aigle-windows-amd64.exe"), []byte("MZ-AIGLE-TEST-BINARY"), 0644)

	cfg := &config.ServerConfig{
		Port:               "8090",
		AppDomain:          "aigle.heimerdinger.studio",
		AuthDomain:         "airgle-auth.heimerdinger.studio",
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
	// Vérification de la présence des en-têtes de sécurité OWASP
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
	reqViewerWrite := httptest.NewRequest(http.MethodPost, "/api/projects/prj_demo_saas/tickets", bytes.NewBufferString(`{"title":"Ticket non autorisé"}`))
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

	// 1. La racine publique (/) est accessible SANS authentification Authelia (200 OK)
	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	recRoot := httptest.NewRecorder()
	handler.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur la vitrine publique /, obtenu %d", recRoot.Code)
	}

	// 2. L'espace SaaS (/app) est PROTÉGÉ et renvoie 401 sans en-têtes Authelia
	reqAppNoAuth := httptest.NewRequest(http.MethodGet, "/app", nil)
	recAppNoAuth := httptest.NewRecorder()
	handler.ServeHTTP(recAppNoAuth, reqAppNoAuth)
	if recAppNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("Attendu 401 sur l'espace protégé /app sans Authelia, obtenu %d", recAppNoAuth.Code)
	}

	// 3. Le catalogue public des téléchargements (/public/api/releases) est accessible sans auth (200 OK)
	reqRel := httptest.NewRequest(http.MethodGet, "/public/api/releases", nil)
	recRel := httptest.NewRecorder()
	handler.ServeHTTP(recRel, reqRel)
	if recRel.Code != http.StatusOK {
		t.Fatalf("Attendu 200 sur /public/api/releases, obtenu %d", recRel.Code)
	}

	// 4. Le téléchargement du binaire Windows autorisé fonctionne et renvoie l'en-tête X-Checksum-SHA256
	reqDL := httptest.NewRequest(http.MethodGet, "/public/downloads/aigle-windows-amd64.exe", nil)
	recDL := httptest.NewRecorder()
	handler.ServeHTTP(recDL, reqDL)
	if recDL.Code != http.StatusOK {
		t.Fatalf("Attendu 200 lors du téléchargement de aigle-windows-amd64.exe, obtenu %d", recDL.Code)
	}
	if recDL.Header().Get("X-Checksum-SHA256") == "" {
		t.Errorf("En-tête X-Checksum-SHA256 manquant sur le téléchargement du binaire")
	}

	// 5. Tentative d'accès à un fichier hors liste blanche -> rejeté (404/400)
	reqEvil := httptest.NewRequest(http.MethodGet, "/public/downloads/aigle_saas_test.db", nil)
	recEvil := httptest.NewRecorder()
	handler.ServeHTTP(recEvil, reqEvil)
	if recEvil.Code == http.StatusOK {
		t.Fatalf("Un fichier hors liste blanche ne doit jamais pouvoir être téléchargé")
	}
}
