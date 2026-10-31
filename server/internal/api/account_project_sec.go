package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aigle/aigle-server/internal/authelia"
	"github.com/aigle/aigle-server/internal/middleware"
	"github.com/aigle/aigle-server/internal/models"
)

// ============================================================================
// 1. GESTION DU COMPTE PERSONNEL, MOT DE PASSE & 2FA TOTP (TODO #1 & #3)
// ============================================================================

func (s *Server) isTOTPVerifiedForSession(u *models.User) bool {
	if u == nil || u.TOTPVerifiedAt.IsZero() {
		return false
	}
	ttl := time.Duration(s.Cfg.TOTPSessionMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return time.Since(u.TOTPVerifiedAt) < ttl
}

// handleGetAccount retourne les informations du compte de l'utilisateur connecté,
// son statut d'enrôlement 2FA TOTP et les modes de changement de mot de passe disponibles.
func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	freshUser, err := s.Store.GetUserByID(u.ID)
	if err == nil && freshUser != nil {
		u = freshUser
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":                   u,
		"totp_enabled":           u.TOTPEnabled,
		"totp_verified_session":  s.isTOTPVerifiedForSession(u),
		"password_mode":          s.Cfg.PasswordMode,
		"allow_direct_password":  s.Cfg.PasswordMode == "direct" || s.Cfg.PasswordMode == "both",
		"allow_authelia_portal":  s.Cfg.PasswordMode == "authelia_smtp" || s.Cfg.PasswordMode == "both",
		"authelia_settings_url":  fmt.Sprintf("https://%s/settings", s.Cfg.AuthDomain),
		"totp_session_minutes":   s.Cfg.TOTPSessionMinutes,
	})
}

// handleUpdateAccountProfile permet à l'utilisateur de modifier son nom complet d'affichage.
func (s *Server) handleUpdateAccountProfile(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	var payload struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(payload.DisplayName)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "Le nom d'affichage ne peut pas être vide.")
		return
	}
	if err := s.Store.UpdateUserProfile(u.ID, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ACCOUNT_PROFILE_UPDATE", "user", u.ID, fmt.Sprintf("Mise à jour du profil par l'utilisateur (%s)", name))
	u.DisplayName = name
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "message": "Profil mis à jour avec succès."})
}

// handleChangePassword permet à l'utilisateur connecté de changer son mot de passe.
// Selon la configuration (PasswordMode = "both" ou "direct"), vérifie l'ancien mot de passe
// et met à jour le hash Argon2id dans users_database.yml d'Authelia + dans la base SQL.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if s.Cfg.PasswordMode == "authelia_smtp" {
		writeErr(w, http.StatusForbidden, fmt.Sprintf("Le changement de mot de passe est configuré en mode Portail Authelia SMTP uniquement (https://%s/settings).", s.Cfg.AuthDomain))
		return
	}

	var payload struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
		ConfirmPassword string `json:"confirm_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(payload.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "Le nouveau mot de passe doit contenir au moins 8 caractères.")
		return
	}
	if payload.ConfirmPassword != "" && payload.NewPassword != payload.ConfirmPassword {
		writeErr(w, http.StatusBadRequest, "La confirmation du nouveau mot de passe ne correspond pas.")
		return
	}

	freshUser, _ := s.Store.GetUserByID(u.ID)
	if freshUser != nil {
		u = freshUser
	}

	// 1. Vérification du mot de passe actuel (depuis users_database.yml d'Authelia ou depuis la DB)
	verified := false
	fileHash, fileErr := authelia.ReadUserPasswordHashFromFile(s.Cfg.AutheliaUsersFile, u.Username, u.Email)
	if fileErr == nil && fileHash != "" {
		if authelia.VerifyPasswordArgon2id(payload.CurrentPassword, fileHash) {
			verified = true
		}
	} else if u.PasswordHash != "" {
		if authelia.VerifyPasswordArgon2id(payload.CurrentPassword, u.PasswordHash) {
			verified = true
		}
	} else if payload.CurrentPassword != "" {
		// Premier changement en environnement de test/dev où aucun hash initial n'était présent en DB
		verified = true
	}

	if !verified {
		s.audit(r, "ACCOUNT_PASSWORD_FAILED", "user", u.ID, "Échec de changement de mot de passe : mot de passe actuel incorrect")
		writeErr(w, http.StatusUnauthorized, "Le mot de passe actuel saisi est incorrect.")
		return
	}

	// 2. Génération de la nouvelle empreinte Argon2id compatible Authelia v4
	newHash, err := authelia.HashPasswordArgon2id(payload.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Erreur de hachage Argon2id : "+err.Error())
		return
	}

	// 3. Mise à jour en base de données SQL
	if err := s.Store.UpdateUserPasswordHash(u.ID, newHash); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 4. Synchronisation immédiate dans users_database.yml d'Authelia (rechargé à chaud par watch: true)
	autheliaSynced := false
	if err := authelia.UpdateUserPasswordInFile(s.Cfg.AutheliaUsersFile, u.Username, u.Email, u.DisplayName, u.Role, newHash); err == nil {
		autheliaSynced = true
	}

	s.audit(r, "ACCOUNT_PASSWORD_CHANGE", "user", u.ID, fmt.Sprintf("Changement de mot de passe Argon2id réussi (sync Authelia=%v)", autheliaSynced))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"authelia_synced": autheliaSynced,
		"message":         "Votre mot de passe a été modifié avec succès (Argon2id).",
	})
}

// handleSetupTOTP génère un secret TOTP RFC 6238 et renvoie le QR Code SVG autonome pour enrôlement.
func (s *Server) handleSetupTOTP(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	secret, err := authelia.GenerateTOTPSecret()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Erreur génération secret TOTP : "+err.Error())
		return
	}
	// Enregistre le secret en attente de validation (enabled = false jusqu'au premier code validé)
	if err := s.Store.SaveUserTOTPSecret(u.ID, secret, false); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	otpURI := authelia.BuildOTPAuthURI("Aigle SaaS", u.Email, secret)
	qrSVG := authelia.GenerateQRCodeDataURI(otpURI)

	s.audit(r, "ACCOUNT_TOTP_SETUP_INIT", "user", u.ID, "Génération d'une nouvelle clé d'enrôlement 2FA TOTP")
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_uri": otpURI,
		"qr_svg":      qrSVG,
		"issuer":      "Aigle SaaS",
		"account":     u.Email,
	})
}

// handleActivateTOTP vérifie le premier code à 6 chiffres et active définitivement la 2FA TOTP sur le compte.
func (s *Server) handleActivateTOTP(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	fresh, err := s.Store.GetUserByID(u.ID)
	if err != nil || fresh == nil || fresh.TOTPSecret == "" {
		writeErr(w, http.StatusBadRequest, "Aucun enrôlement TOTP en cours. Veuillez d'abord générer un QR Code.")
		return
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if !authelia.VerifyTOTPCode(fresh.TOTPSecret, payload.Code, time.Now()) {
		s.audit(r, "ACCOUNT_TOTP_ACTIVATE_FAILED", "user", u.ID, "Code TOTP à 6 chiffres invalide lors de l'activation")
		writeErr(w, http.StatusUnauthorized, "Code TOTP invalide. Vérifiez l'heure de votre téléphone et réessayez.")
		return
	}

	if err := s.Store.SaveUserTOTPSecret(u.ID, fresh.TOTPSecret, true); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ACCOUNT_TOTP_ACTIVATED", "user", u.ID, "Double authentification 2FA TOTP activée avec succès sur le compte")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"totp_enabled":          true,
		"totp_verified_session": true,
		"message":               "Double authentification 2FA TOTP activée et session vérifiée.",
	})
}

// handleDisableTOTP désactive la 2FA TOTP du compte après vérification d'un code TOTP valide.
func (s *Server) handleDisableTOTP(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	fresh, err := s.Store.GetUserByID(u.ID)
	if err != nil || fresh == nil {
		writeErr(w, http.StatusNotFound, "Utilisateur introuvable")
		return
	}

	var payload struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)

	if fresh.TOTPEnabled && fresh.TOTPSecret != "" {
		if !authelia.VerifyTOTPCode(fresh.TOTPSecret, payload.Code, time.Now()) {
			writeErr(w, http.StatusUnauthorized, "Code TOTP à 6 chiffres requis et valide pour désactiver la 2FA.")
			return
		}
	}

	if err := s.Store.SaveUserTOTPSecret(u.ID, "", false); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ACCOUNT_TOTP_DISABLED", "user", u.ID, "Désactivation de la 2FA TOTP sur le compte")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"totp_enabled": false,
		"message":      "La double authentification 2FA TOTP a été désactivée sur votre compte.",
	})
}

// handleVerifyProjectTOTP vérifie un code TOTP à 6 chiffres pour déverrouiller un projet protégé par 2FA.
func (s *Server) handleVerifyProjectTOTP(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	projectID := r.PathValue("id")

	fresh, err := s.Store.GetUserByID(u.ID)
	if err != nil || fresh == nil {
		writeErr(w, http.StatusUnauthorized, "Utilisateur introuvable")
		return
	}
	if !fresh.TOTPEnabled || fresh.TOTPSecret == "" {
		writeErr(w, http.StatusBadRequest, "Vous devez d'abord activer votre 2FA TOTP dans « Mon Compte » avant d'accéder à ce projet protégé.")
		return
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now()
	if !authelia.VerifyTOTPCode(fresh.TOTPSecret, payload.Code, now) {
		s.audit(r, "PROJECT_TOTP_FAILED", "project", projectID, "Code 2FA TOTP invalide lors du déverrouillage du projet")
		writeErr(w, http.StatusUnauthorized, "Code TOTP à 6 chiffres incorrect ou expiré.")
		return
	}

	if err := s.Store.MarkUserTOTPVerified(u.ID, now); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.audit(r, "PROJECT_TOTP_VERIFIED", "project", projectID, "Déverrouillage 2FA TOTP réussi pour le projet")
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"project_id":            projectID,
		"totp_verified_session": true,
		"valid_for_minutes":     s.Cfg.TOTPSessionMinutes,
	})
}

// ============================================================================
// 2. GESTION DES MAÎTRES DE PROJET, DROITS FINS & SÉCURITÉ 2FA PROJET (TODO #2 & #3)
// ============================================================================

// syncAutheliaTwoFactorProjects régénère le bloc de règles two_factor par projet dans configuration.yml d'Authelia.
func (s *Server) syncAutheliaTwoFactorProjects() {
	ids, err := s.Store.GetTOTPRequiredProjectIDs()
	if err == nil {
		_ = authelia.SyncProjectTwoFactorRules(s.Cfg.AutheliaConfigFile, s.Cfg.AppDomain, ids)
	}
}

// checkProjectAccess vérifie :
//  1. L'existence du projet et l'isolation Multi-Tenant (WorkspaceID)
//  2. Si le projet exige la double authentification 2FA TOTP (RequireTOTP), vérifie que la session TOTP de l'utilisateur est active
//  3. Retourne le projet et les permissions granulaires effectives de l'utilisateur sur ce projet
func (s *Server) checkProjectAccess(w http.ResponseWriter, r *http.Request, projectID string) (*models.Project, models.ProjectMember, bool) {
	u := middleware.GetUser(r)
	proj, err := s.Store.GetProject(projectID)
	if err != nil || proj == nil {
		writeErr(w, http.StatusNotFound, "Projet introuvable")
		return nil, models.ProjectMember{}, false
	}

	// 1. Isolation Multi-Tenant stricte
	if proj.WorkspaceID != u.WorkspaceID && !middleware.IsSuperAdmin(u) {
		s.audit(r, "CROSS_TENANT_BLOCKED", "project", projectID, fmt.Sprintf("Tentative d'accès hors Workspace (%s -> %s)", u.WorkspaceID, proj.WorkspaceID))
		writeErr(w, http.StatusForbidden, "Accès refusé : ce projet appartient à un autre espace client (Workspace).")
		return nil, models.ProjectMember{}, false
	}

	// 2. Résolution des permissions fines de l'utilisateur sur ce projet
	perm := s.Store.ResolveProjectPermissions(u, proj)
	if !perm.CanRead {
		s.audit(r, "PROJECT_READ_DENIED", "project", projectID, "Accès en lecture refusé sur ce projet")
		writeErr(w, http.StatusForbidden, "Accès refusé : vous n'avez pas le droit de lecture sur ce projet.")
		return nil, perm, false
	}

	// 3. Vérification de la double authentification 2FA TOTP si le projet l'exige
	if proj.RequireTOTP {
		freshUser, err := s.Store.GetUserByID(u.ID)
		if err == nil && freshUser != nil {
			u = freshUser
		}
		if !s.isTOTPVerifiedForSession(u) {
			writeJSON(w, http.StatusPreconditionRequired, map[string]any{
				"error":        fmt.Sprintf("Le projet « %s » exige la double authentification (2FA TOTP). Veuillez valider votre code à 6 chiffres.", proj.Name),
				"code":         "TOTP_REQUIRED",
				"project_id":   proj.ID,
				"project_code": proj.Code,
				"project_name": proj.Name,
				"totp_enabled": u.TOTPEnabled,
			})
			return nil, perm, false
		}
	}

	return proj, perm, true
}

// handleGetWorkspaceUsers retourne les utilisateurs disponibles dans le Workspace actif
// afin de pouvoir les désigner comme Maîtres du Projet ou membres avec permissions fines.
func (s *Server) handleGetWorkspaceUsers(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	allUsers, err := s.Store.GetUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var filtered []models.User
	for _, usr := range allUsers {
		if middleware.IsSuperAdmin(u) || usr.WorkspaceID == u.WorkspaceID || usr.Role == models.RoleSuperAdmin {
			filtered = append(filtered, usr)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

// handleGetProjectMembers retourne les membres du projet, leurs permissions fines et la configuration 2FA du projet.
func (s *Server) handleGetProjectMembers(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	proj, perm, ok := s.checkProjectAccess(w, r, projectID)
	if !ok {
		return
	}

	members, err := s.Store.GetProjectMembers(projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project_id":     proj.ID,
		"owner_user_id":  proj.OwnerUserID,
		"owner_name":     proj.OwnerName,
		"require_totp":   proj.RequireTOTP,
		"my_permissions": perm,
		"members":        members,
	})
}

// handleSaveProjectMember ajoute ou modifie un utilisateur dans l'équipe du projet avec ses 8 permissions fines.
func (s *Server) handleSaveProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	proj, perm, ok := s.checkProjectAccess(w, r, projectID)
	if !ok {
		return
	}
	if !perm.CanManageMembers {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants : seul un Maître du Projet ou un Administrateur peut gérer les membres et leurs droits.")
		return
	}

	var payload struct {
		models.ProjectMember
		ApplyPreset bool `json:"apply_preset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	pm := payload.ProjectMember
	pm.ProjectID = projectID
	if strings.TrimSpace(pm.UserID) == "" {
		writeErr(w, http.StatusBadRequest, "Veuillez sélectionner un utilisateur.")
		return
	}

	if payload.ApplyPreset {
		preset := models.PresetPermissionsForRole(pm.ProjectRole)
		preset.ID = pm.ID
		preset.ProjectID = projectID
		preset.UserID = pm.UserID
		preset.UserEmail = pm.UserEmail
		preset.UserName = pm.UserName
		pm = preset
	}

	if err := s.Store.SaveProjectMember(&pm); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Si l'utilisateur est défini comme Maître du Projet ('owner') et que le projet n'avait pas encore de Maître principal
	if pm.ProjectRole == models.ProjectRoleOwner && proj.OwnerUserID == "" {
		proj.OwnerUserID = pm.UserID
		_ = s.Store.UpdateProject(proj)
	}

	s.audit(r, "PROJECT_MEMBER_SAVE", "project", projectID, fmt.Sprintf("Attribution droits projet à %s : rôle=%s (read=%v, write_tck=%v, del_tck=%v, exec=%v, gantt=%v, wiki=%v, fin=%v, mgr=%v)",
		pm.UserEmail, pm.ProjectRole, pm.CanRead, pm.CanWriteTickets, pm.CanDeleteTickets, pm.CanExecute, pm.CanEditGantt, pm.CanEditWiki, pm.CanViewFinance, pm.CanManageMembers))
	s.broadcastChange(proj.WorkspaceID, "project_member_updated", projectID)
	writeJSON(w, http.StatusOK, pm)
}

// handleDeleteProjectMember retire un utilisateur de l'équipe d'un projet.
func (s *Server) handleDeleteProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	proj, perm, ok := s.checkProjectAccess(w, r, projectID)
	if !ok {
		return
	}
	if !perm.CanManageMembers {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants : seul un Maître du Projet peut retirer un membre.")
		return
	}

	targetUserID := r.PathValue("userId")
	if err := s.Store.DeleteProjectMember(projectID, targetUserID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.audit(r, "PROJECT_MEMBER_DELETE", "project", projectID, fmt.Sprintf("Retrait de l'utilisateur %s du projet %s", targetUserID, proj.Code))
	s.broadcastChange(proj.WorkspaceID, "project_member_deleted", projectID)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": targetUserID})
}

// handleUpdateProjectSecurity permet au Maître du Projet ou Admin de modifier le Maître principal
// et d'activer/désactiver l'obligation de double authentification 2FA TOTP sur le projet.
func (s *Server) handleUpdateProjectSecurity(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	projectID := r.PathValue("id")
	proj, perm, ok := s.checkProjectAccess(w, r, projectID)
	if !ok {
		return
	}
	if !perm.CanManageMembers {
		writeErr(w, http.StatusForbidden, "Seul un Maître du Projet ou un Administrateur peut modifier la politique de sécurité 2FA du projet.")
		return
	}

	var payload struct {
		OwnerUserID *string `json:"owner_user_id"`
		RequireTOTP *bool   `json:"require_totp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Si l'utilisateur active RequireTOTP sur le projet, on vérifie qu'il a lui-même activé son TOTP
	// (ou on marque sa session comme vérifiée s'il l'a déjà activé) pour éviter qu'il ne s'auto-bloque.
	if payload.RequireTOTP != nil && *payload.RequireTOTP {
		freshUser, _ := s.Store.GetUserByID(u.ID)
		if freshUser == nil || !freshUser.TOTPEnabled {
			writeErr(w, http.StatusBadRequest, "Pour activer la 2FA TOTP obligatoire sur ce projet, veuillez d'abord configurer votre propre 2FA TOTP dans « 👤 Mon Compte ».")
			return
		}
		_ = s.Store.MarkUserTOTPVerified(u.ID, time.Now())
	}

	if payload.OwnerUserID != nil && *payload.OwnerUserID != "" {
		proj.OwnerUserID = *payload.OwnerUserID
		ownerMember := models.PresetPermissionsForRole(models.ProjectRoleOwner)
		ownerMember.ProjectID = proj.ID
		ownerMember.UserID = *payload.OwnerUserID
		_ = s.Store.SaveProjectMember(&ownerMember)
	}
	if payload.RequireTOTP != nil {
		proj.RequireTOTP = *payload.RequireTOTP
	}

	if err := s.Store.UpdateProject(proj); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Synchronise immédiatement la règle two_factor dans configuration.yml d'Authelia
	s.syncAutheliaTwoFactorProjects()

	s.audit(r, "PROJECT_SECURITY_UPDATE", "project", proj.ID, fmt.Sprintf("Mise à jour sécurité projet %s : owner=%s, require_totp=%v", proj.Code, proj.OwnerUserID, proj.RequireTOTP))
	s.broadcastChange(proj.WorkspaceID, "project_security_updated", proj.ID)
	writeJSON(w, http.StatusOK, proj)
}

