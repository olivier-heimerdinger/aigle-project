// Package api implémente le serveur HTTP REST et SSE (Server-Sent Events) d'Aigle Server (Édition SaaS).
// Il assure :
//   - La validation Zero-Trust de l'identité SSO Authelia transmise par Traefik (ForwardAuth)
//   - Le contrôle d'accès basé sur les rôles (RBAC : superadmin, admin, manager, member, viewer)
//   - L'isolation stricte des espaces de travail Multi-Tenant (Workspaces)
//   - La Console et l'API d'Administration SaaS (/api/admin/...)
//   - La journalisation immuable des événements de sécurité (audit_logs)
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aigle/aigle-server/internal/config"
	"github.com/aigle/aigle-server/internal/database"
	"github.com/aigle/aigle-server/internal/engine"
	"github.com/aigle/aigle-server/internal/middleware"
	"github.com/aigle/aigle-server/internal/models"
)

type sseClient struct {
	ch          chan string
	workspaceID string
	isSuper     bool
}

// Server regroupe le magasin de données SQL, la configuration SaaS, le routeur HTTP et le hub SSE.
type Server struct {
	Store   *database.Store
	Cfg     *config.ServerConfig
	WebFS   fs.FS
	Limiter *middleware.RateLimiter

	sseMu      sync.Mutex
	sseClients map[*sseClient]bool
}

// NewServer construit et configure le serveur HTTP Aigle SaaS avec toute sa chaîne de sécurité.
func NewServer(store *database.Store, cfg *config.ServerConfig, webFS fs.FS) http.Handler {
	s := &Server{
		Store:      store,
		Cfg:        cfg,
		WebFS:      webFS,
		Limiter:    middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst),
		sseClients: make(map[*sseClient]bool),
	}

	mux := http.NewServeMux()

	// --- Sonde de santé publique (utilisée par Docker/Podman/K8s HEALTHCHECK et Traefik) ---
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// --- Zone Publique (Vitrine de Présentation & Centre de Téléchargement Windows / macOS / Linux) ---
	mux.HandleFunc("GET /public/api/releases", s.handlePublicReleases)
	mux.HandleFunc("GET /public/downloads/{filename}", s.handlePublicDownloadFile)

	// --- Profil SSO Authelia & Contexte Multi-Tenant (Zone Protégée) ---
	mux.HandleFunc("GET /api/me", s.withAuth(s.handleGetMe))

	// --- Console d'Administration SaaS (/api/admin/...) réservée aux SuperAdmins / Admins ---
	mux.HandleFunc("GET /api/admin/overview", s.withAuth(s.handleAdminOverview))
	mux.HandleFunc("GET /api/admin/workspaces", s.withAuth(s.handleAdminGetWorkspaces))
	mux.HandleFunc("POST /api/admin/workspaces", s.withAuth(s.handleAdminSaveWorkspace))
	mux.HandleFunc("PUT /api/admin/workspaces/{id}", s.withAuth(s.handleAdminUpdateWorkspace))
	mux.HandleFunc("GET /api/admin/users", s.withAuth(s.handleAdminGetUsers))
	mux.HandleFunc("POST /api/admin/users", s.withAuth(s.handleAdminSaveUser))
	mux.HandleFunc("PUT /api/admin/users/{id}", s.withAuth(s.handleAdminUpdateUser))
	mux.HandleFunc("GET /api/admin/audit-logs", s.withAuth(s.handleAdminGetAuditLogs))

	// --- Flux temps réel SSE (Server-Sent Events) isolé par Workspace ---
	mux.HandleFunc("GET /api/events", s.withAuth(s.handleSSE))

	// --- Projets Multi-Tenant ---
	mux.HandleFunc("GET /api/projects", s.withAuth(s.handleGetProjects))
	mux.HandleFunc("POST /api/projects", s.withAuth(s.handleCreateProject))
	mux.HandleFunc("PUT /api/projects/{id}", s.withAuth(s.handleUpdateProject))
	mux.HandleFunc("DELETE /api/projects/{id}", s.withAuth(s.handleDeleteProject))
	mux.HandleFunc("GET /api/projects/{id}/full", s.withAuth(s.handleGetProjectFull))

	// --- Intervenants & TJM (Members) ---
	mux.HandleFunc("GET /api/members", s.withAuth(s.handleGetMembers))
	mux.HandleFunc("POST /api/members", s.withAuth(s.handleCreateMember))
	mux.HandleFunc("PUT /api/members/{id}", s.withAuth(s.handleUpdateMember))

	// --- Planning MS Project (WBS & Gantt) ---
	mux.HandleFunc("POST /api/projects/{id}/wbs", s.withAuth(s.handleCreateWBSNode))
	mux.HandleFunc("PUT /api/wbs/{nodeId}", s.withAuth(s.handleUpdateWBSNode))
	mux.HandleFunc("DELETE /api/wbs/{nodeId}", s.withAuth(s.handleDeleteWBSNode))
	mux.HandleFunc("POST /api/projects/{id}/dependencies", s.withAuth(s.handleCreateDependency))
	mux.HandleFunc("DELETE /api/dependencies/{depId}", s.withAuth(s.handleDeleteDependency))

	// --- Tickets Kanban & Sous-tickets Enfants ---
	mux.HandleFunc("POST /api/projects/{id}/tickets", s.withAuth(s.handleCreateTicket))
	mux.HandleFunc("PUT /api/tickets/{ticketId}", s.withAuth(s.handleUpdateTicket))
	mux.HandleFunc("PATCH /api/tickets/{ticketId}/move", s.withAuth(s.handleMoveTicket))
	mux.HandleFunc("DELETE /api/tickets/{ticketId}", s.withAuth(s.handleDeleteTicket))
	mux.HandleFunc("POST /api/tickets/{ticketId}/timelogs", s.withAuth(s.handleAddTimeLog))

	// --- Documentation & Wiki Markdown ---
	mux.HandleFunc("POST /api/projects/{id}/docs", s.withAuth(s.handleCreateDoc))
	mux.HandleFunc("PUT /api/docs/{docId}", s.withAuth(s.handleUpdateDoc))
	mux.HandleFunc("DELETE /api/docs/{docId}", s.withAuth(s.handleDeleteDoc))

	// --- Interface Web SPA Svelte 5 : Zone Publique (/) vs Zone SaaS Protégée (/app) ---
	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		serveSPA := func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" || strings.HasPrefix(path, "app") || strings.HasPrefix(path, "welcome") {
				r.URL.Path = "/"
			} else if _, err := fs.Stat(webFS, path); err != nil {
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		}
		// La route /app (et /app/...) exige l'authentification SSO Authelia
		mux.HandleFunc("GET /app", s.withAuth(serveSPA))
		mux.HandleFunc("GET /app/{rest...}", s.withAuth(serveSPA))
		// La racine / et les assets statiques sont publics pour afficher la vitrine et les téléchargements
		mux.HandleFunc("/", serveSPA)
	} else {
		mux.HandleFunc("GET /app", s.withAuth(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<h1>Aigle SaaS Protected Workspace Active</h1>")
		}))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<h1>Aigle Public Portal & Downloads Active</h1>")
		})
	}

	// Chaîne de sécurité (SecurityHeaders -> RateLimiter -> BodySizeLimit -> Mux)
	var handler http.Handler = mux
	handler = middleware.BodySizeLimit(cfg.MaxBodyBytes, handler)
	handler = s.Limiter.Middleware(handler)
	handler = middleware.SecurityHeaders(handler)

	return handler
}

// withAuth valide l'identité Authelia ForwardAuth, synchronise l'utilisateur en base,
// vérifie que le compte et le Workspace actif ne sont pas suspendus, et gère le changement de Workspace.
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := middleware.ResolveAutheliaIdentity(s.Cfg, r)
		if !ok || identity == nil {
			s.Store.AddAuditLog(
				"ws_default", "anonymous", "none", "AUTH_REJECTED",
				"endpoint", r.URL.Path, "Requête rejetée : en-têtes Authelia absents ou X-Aigle-Proxy-Secret invalide",
				middleware.ExtractClientIP(r),
			)
			writeErr(w, http.StatusUnauthorized, "Authentification SSO Authelia requise (401 Unauthorized).")
			return
		}

		user, err := s.Store.SyncAuthenticatedUser(identity)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "Erreur de synchronisation du profil SSO : "+err.Error())
			return
		}

		if !user.IsActive {
			s.Store.AddAuditLog(
				user.WorkspaceID, user.Email, user.Role, "ACCESS_DENIED_SUSPENDED_USER",
				"user", user.ID, "Tentative de connexion avec un compte suspendu",
				middleware.ExtractClientIP(r),
			)
			writeErr(w, http.StatusForbidden, "Votre compte utilisateur a été suspendu par un administrateur.")
			return
		}

		// Permet à un SuperAdmin / Admin de basculer d'espace client via X-Workspace-ID ou ?workspace_id=
		requestedWS := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
		if requestedWS == "" {
			requestedWS = strings.TrimSpace(r.URL.Query().Get("workspace_id"))
		}
		if requestedWS != "" {
			if middleware.IsSuperAdmin(user) || requestedWS == user.WorkspaceID {
				user.WorkspaceID = requestedWS
			}
		}
		if user.WorkspaceID == "" {
			user.WorkspaceID = "ws_default"
		}

		// Vérifie que le Workspace ciblé n'est pas suspendu (sauf pour les routes /api/admin)
		if !strings.HasPrefix(r.URL.Path, "/api/admin") && user.Role != models.RoleSuperAdmin {
			workspaces, err := s.Store.GetWorkspaces()
			if err == nil {
				for _, ws := range workspaces {
					if ws.ID == user.WorkspaceID && ws.Status == "suspended" {
						writeErr(w, http.StatusForbidden, fmt.Sprintf("L'espace de travail '%s' est actuellement suspendu.", ws.Name))
						return
					}
				}
			}
		}

		next(w, middleware.WithUser(r, user))
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) audit(r *http.Request, action, targetType, targetID, details string) {
	u := middleware.GetUser(r)
	s.Store.AddAuditLog(
		u.WorkspaceID, u.Email, u.Role, action,
		targetType, targetID, details, middleware.ExtractClientIP(r),
	)
}

func (s *Server) broadcastChange(workspaceID, eventType, projectID string) {
	msg := fmt.Sprintf(`{"event":%q,"workspace_id":%q,"project_id":%q,"ts":%d}`,
		eventType, workspaceID, projectID, time.Now().UnixMilli())
	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	for client := range s.sseClients {
		if client.isSuper || client.workspaceID == workspaceID {
			select {
			case client.ch <- msg:
			default:
			}
		}
	}
}

// --- Sonde de Santé & Profil SSO ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DB.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"app":         "Aigle SaaS Server",
		"mode":        "saas",
		"auth_mode":   s.Cfg.AuthMode,
		"db_driver":   s.Store.Driver,
		"app_domain":  s.Cfg.AppDomain,
		"auth_domain": s.Cfg.AuthDomain,
	})
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	allWS, _ := s.Store.GetWorkspaces()

	var accessible []models.Workspace
	var activeWS *models.Workspace
	for i := range allWS {
		ws := allWS[i]
		if middleware.IsSuperAdmin(u) || ws.ID == u.WorkspaceID {
			accessible = append(accessible, ws)
		}
		if ws.ID == u.WorkspaceID {
			wsCopy := ws
			activeWS = &wsCopy
		}
	}
	if activeWS == nil && len(accessible) > 0 {
		activeWS = &accessible[0]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":               u,
		"active_workspace":   activeWS,
		"workspaces":         accessible,
		"can_write":          middleware.CanWrite(u),
		"can_manage_project": middleware.CanManageProject(u),
		"is_admin":           middleware.IsSuperAdmin(u),
		"auth_mode":          s.Cfg.AuthMode,
		"app_domain":         s.Cfg.AppDomain,
		"auth_domain":        s.Cfg.AuthDomain,
		"logout_url":         fmt.Sprintf("https://%s/logout", s.Cfg.AuthDomain),
	})
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE non supporté", http.StatusInternalServerError)
		return
	}
	u := middleware.GetUser(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := &sseClient{
		ch:          make(chan string, 16),
		workspaceID: u.WorkspaceID,
		isSuper:     middleware.IsSuperAdmin(u),
	}
	s.sseMu.Lock()
	s.sseClients[client] = true
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, client)
		close(client.ch)
		s.sseMu.Unlock()
	}()

	fmt.Fprintf(w, "data: {\"event\":\"connected\",\"workspace_id\":%q}\n\n", u.WorkspaceID)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-client.ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// ============================================================================
// CONSOLE D'ADMINISTRATION SAAS (/api/admin/...)
// ============================================================================

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	u := middleware.GetUser(r)
	if !middleware.IsSuperAdmin(u) {
		s.audit(r, "ADMIN_ACCESS_DENIED", "endpoint", r.URL.Path, "Tentative d'accès non autorisé à la Console d'Administration")
		writeErr(w, http.StatusForbidden, "Accès refusé : privilèges d'administrateur SaaS requis.")
		return nil, false
	}
	return u, true
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}

	workspaces, err := s.Store.GetWorkspaces()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	users, err := s.Store.GetUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logs, err := s.Store.GetRecentAuditLogs(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	var totalProjects, totalTickets int
	var totalBudget float64
	for _, ws := range workspaces {
		totalProjects += ws.ProjectCount
		totalTickets += ws.TicketCount
		totalBudget += ws.TotalBudget
	}

	overview := models.SystemOverview{
		AppVersion:      "2.0.0-saas",
		AuthMode:        s.Cfg.AuthMode,
		DBDriver:        s.Store.Driver,
		AppDomain:       s.Cfg.AppDomain,
		AuthDomain:      s.Cfg.AuthDomain,
		TotalWorkspaces: len(workspaces),
		TotalUsers:      len(users),
		TotalProjects:   totalProjects,
		TotalTickets:    totalTickets,
		TotalBudget:     totalBudget,
		Workspaces:      workspaces,
		Users:           users,
		RecentAuditLogs: logs,
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleAdminGetWorkspaces(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	ws, err := s.Store.GetWorkspaces()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (s *Server) handleAdminSaveWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var ws models.Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(ws.Name) == "" {
		writeErr(w, http.StatusBadRequest, "Le nom du Workspace est obligatoire")
		return
	}
	if strings.TrimSpace(ws.Slug) == "" {
		ws.Slug = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(ws.Name), " ", "-"))
	}
	if err := s.Store.SaveWorkspace(&ws); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ADMIN_WORKSPACE_CREATE", "workspace", ws.ID, fmt.Sprintf("Création du Workspace '%s' (slug=%s, quotas=%d projets/%d users)", ws.Name, ws.Slug, ws.MaxProjects, ws.MaxUsers))
	writeJSON(w, http.StatusCreated, ws)
}

func (s *Server) handleAdminUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var ws models.Workspace
	if err := json.NewDecoder(r.Body).Decode(&ws); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ws.ID = id
	if err := s.Store.SaveWorkspace(&ws); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ADMIN_WORKSPACE_UPDATE", "workspace", ws.ID, fmt.Sprintf("Mise à jour Workspace '%s' (statut=%s, quotas=%d projets/%d users)", ws.Name, ws.Status, ws.MaxProjects, ws.MaxUsers))
	writeJSON(w, http.StatusOK, ws)
}

func (s *Server) handleAdminGetUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	users, err := s.Store.GetUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleAdminSaveUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	var u models.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(u.Email) == "" {
		writeErr(w, http.StatusBadRequest, "L'adresse email est obligatoire")
		return
	}
	if u.Username == "" {
		u.Username = strings.Split(u.Email, "@")[0]
	}
	if u.WorkspaceID == "" {
		u.WorkspaceID = "ws_default"
	}
	if u.Role == "" {
		u.Role = models.RoleMember
	}
	u.IsActive = true
	if err := s.Store.SaveUser(&u); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ADMIN_USER_CREATE", "user", u.ID, fmt.Sprintf("Pré-enregistrement SSO de %s (rôle=%s, workspace=%s)", u.Email, u.Role, u.WorkspaceID))
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var u models.User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u.ID = id
	if err := s.Store.SaveUser(&u); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "ADMIN_USER_UPDATE", "user", u.ID, fmt.Sprintf("Modification RBAC utilisateur %s : rôle=%s, workspace=%s, actif=%v", u.Email, u.Role, u.WorkspaceID, u.IsActive))
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleAdminGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	logs, err := s.Store.GetRecentAuditLogs(200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// ============================================================================
// PROJETS, MEMBRES, WBS/GANTT, KANBAN & WIKI (ISOLÉS PAR WORKSPACE)
// ============================================================================

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	projects, err := s.Store.GetProjects(u.WorkspaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	members, _ := s.Store.GetMembers(u.WorkspaceID)
	for _, p := range projects {
		nodes, _ := s.Store.GetWBSNodes(p.ID)
		deps, _ := s.Store.GetWBSDependencies(p.ID)
		tickets, _ := s.Store.GetTickets(p.ID)
		logs, _ := s.Store.GetTimeLogsByProject(p.ID)
		_ = engine.ComputeSchedule(nodes, deps)
		engine.ComputeProjectFinancials(p, nodes, tickets, members, logs)
	}
	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanManageProject(u) {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants : rôle 'manager' ou 'admin' requis pour créer un projet.")
		return
	}
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p.WorkspaceID = u.WorkspaceID
	if err := s.Store.CreateProject(&p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "PROJECT_CREATE", "project", p.ID, fmt.Sprintf("Création du projet %s (%s) dans %s", p.Code, p.Name, p.WorkspaceID))
	s.broadcastChange(u.WorkspaceID, "project_created", p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanManageProject(u) {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants pour modifier la configuration du projet.")
		return
	}
	id := r.PathValue("id")
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p.ID = id
	p.WorkspaceID = u.WorkspaceID
	if err := s.Store.UpdateProject(&p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "PROJECT_UPDATE", "project", p.ID, fmt.Sprintf("Modification du projet %s (Budget=%.0f)", p.Code, p.InitialBudget))
	s.broadcastChange(u.WorkspaceID, "project_updated", p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanManageProject(u) {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants pour supprimer un projet.")
		return
	}
	id := r.PathValue("id")
	if err := s.Store.DeleteProject(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "PROJECT_DELETE", "project", id, "Suppression complète du projet")
	s.broadcastChange(u.WorkspaceID, "project_deleted", id)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

func (s *Server) handleGetProjectFull(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	id := r.PathValue("id")
	proj, err := s.Store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Projet introuvable")
		return
	}
	// Isolation Multi-Tenant : interdiction de lire un projet appartenant à un autre Workspace
	if proj.WorkspaceID != u.WorkspaceID && !middleware.IsSuperAdmin(u) {
		s.audit(r, "CROSS_TENANT_BLOCKED", "project", id, fmt.Sprintf("Tentative d'accès hors Workspace (%s -> %s)", u.WorkspaceID, proj.WorkspaceID))
		writeErr(w, http.StatusForbidden, "Accès refusé : ce projet appartient à un autre espace client (Workspace).")
		return
	}

	nodes, err := s.Store.GetWBSNodes(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	deps, err := s.Store.GetWBSDependencies(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	tickets, err := s.Store.GetTickets(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	logs, err := s.Store.GetTimeLogsByProject(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	docs, err := s.Store.GetDocPages(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	members, err := s.Store.GetMembers(proj.WorkspaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	scheduleErr := engine.ComputeSchedule(nodes, deps)
	finSummary := engine.ComputeProjectFinancials(proj, nodes, tickets, members, logs)

	resp := map[string]any{
		"project":      proj,
		"financials":   finSummary,
		"wbs_nodes":    nodes,
		"dependencies": deps,
		"tickets":      tickets,
		"time_logs":    logs,
		"docs":         docs,
		"members":      members,
	}
	if scheduleErr != nil {
		resp["schedule_warning"] = scheduleErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetMembers(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	members, err := s.Store.GetMembers(u.WorkspaceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanManageProject(u) {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants pour ajouter un intervenant.")
		return
	}
	var m models.Member
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m.WorkspaceID = u.WorkspaceID
	if err := s.Store.CreateMember(&m); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "MEMBER_CREATE", "member", m.ID, fmt.Sprintf("Ajout intervenant %s (%.0f€/h)", m.Name, m.HourlyRate))
	s.broadcastChange(u.WorkspaceID, "member_created", "")
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanManageProject(u) {
		writeErr(w, http.StatusForbidden, "Privilèges insuffisants pour modifier un intervenant.")
		return
	}
	id := r.PathValue("id")
	var m models.Member
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m.ID = id
	m.WorkspaceID = u.WorkspaceID
	if err := s.Store.UpdateMember(&m); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "MEMBER_UPDATE", "member", m.ID, fmt.Sprintf("Mise à jour TJM de %s (%.0f€/h)", m.Name, m.HourlyRate))
	s.broadcastChange(u.WorkspaceID, "member_updated", "")
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleCreateWBSNode(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule (rôle 'viewer') : modification du planning non autorisée.")
		return
	}
	projectID := r.PathValue("id")
	var n models.WBSNode
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n.ProjectID = projectID
	if err := s.Store.SaveWBSNode(&n); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "wbs_created", projectID)
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateWBSNode(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : modification du planning non autorisée.")
		return
	}
	nodeID := r.PathValue("nodeId")
	var n models.WBSNode
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	n.ID = nodeID
	if err := s.Store.SaveWBSNode(&n); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "wbs_updated", n.ProjectID)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteWBSNode(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : suppression non autorisée.")
		return
	}
	nodeID := r.PathValue("nodeId")
	if err := s.Store.DeleteWBSNode(nodeID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "wbs_deleted", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": nodeID})
}

func (s *Server) handleCreateDependency(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : ajout de dépendance non autorisé.")
		return
	}
	projectID := r.PathValue("id")
	var d models.WBSDependency
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ProjectID = projectID
	if d.PredecessorID == d.SuccessorID {
		writeErr(w, http.StatusBadRequest, "Une tâche ne peut pas dépendre d'elle-même")
		return
	}
	nodes, _ := s.Store.GetWBSNodes(projectID)
	deps, _ := s.Store.GetWBSDependencies(projectID)
	testDeps := append(deps, d)
	if err := engine.ComputeSchedule(nodes, testDeps); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.Store.CreateWBSDependency(&d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "dependency_created", projectID)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleDeleteDependency(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : suppression de dépendance non autorisée.")
		return
	}
	depID := r.PathValue("depId")
	if err := s.Store.DeleteWBSDependency(depID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "dependency_deleted", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": depID})
}

func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule (rôle 'viewer') : création de ticket non autorisée.")
		return
	}
	projectID := r.PathValue("id")
	var t models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	t.ProjectID = projectID
	if err := s.Store.SaveTicket(&t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "TICKET_CREATE", "ticket", t.ID, fmt.Sprintf("Création du ticket %s : %s", t.TicketKey, t.Title))
	s.broadcastChange(u.WorkspaceID, "ticket_created", projectID)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : modification de ticket non autorisée.")
		return
	}
	ticketID := r.PathValue("ticketId")
	var t models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	t.ID = ticketID
	if err := s.Store.SaveTicket(&t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "ticket_updated", t.ProjectID)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleMoveTicket(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : déplacement de ticket non autorisé.")
		return
	}
	ticketID := r.PathValue("ticketId")
	existing, err := s.Store.GetTicket(ticketID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ticket introuvable")
		return
	}

	var payload struct {
		Status         string   `json:"status"`
		WBSNodeID      *string  `json:"wbs_node_id"`
		ParentTicketID *string  `json:"parent_ticket_id"`
		RemainingHours *float64 `json:"remaining_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if payload.Status != "" {
		existing.Status = payload.Status
		if payload.Status == "done" {
			existing.RemainingHours = 0
		}
	}
	if payload.WBSNodeID != nil {
		existing.WBSNodeID = *payload.WBSNodeID
	}
	if payload.ParentTicketID != nil && *payload.ParentTicketID != existing.ID {
		existing.ParentTicketID = *payload.ParentTicketID
	}
	if payload.RemainingHours != nil {
		existing.RemainingHours = *payload.RemainingHours
	}

	if err := s.Store.SaveTicket(existing); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "ticket_moved", existing.ProjectID)
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : suppression de ticket non autorisée.")
		return
	}
	ticketID := r.PathValue("ticketId")
	if err := s.Store.DeleteTicket(ticketID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "TICKET_DELETE", "ticket", ticketID, "Suppression d'un ticket")
	s.broadcastChange(u.WorkspaceID, "ticket_deleted", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": ticketID})
}

func (s *Server) handleAddTimeLog(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : pointage d'heures non autorisé.")
		return
	}
	ticketID := r.PathValue("ticketId")
	var payload struct {
		MemberID          string   `json:"member_id"`
		Hours             float64  `json:"hours"`
		WorkDate          string   `json:"work_date"`
		Comment           string   `json:"comment"`
		RemainingHours    *float64 `json:"remaining_hours"`
		NewRemainingHours *float64 `json:"new_remaining_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rem := payload.NewRemainingHours
	if rem == nil {
		rem = payload.RemainingHours
	}

	logEntry := models.TimeLog{
		TicketID: ticketID,
		MemberID: payload.MemberID,
		Hours:    payload.Hours,
		WorkDate: payload.WorkDate,
		Comment:  payload.Comment,
	}
	if err := s.Store.AddTimeLog(&logEntry, rem); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "timelog_added", "")
	writeJSON(w, http.StatusCreated, logEntry)
}

func (s *Server) handleCreateDoc(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : création de page Wiki non autorisée.")
		return
	}
	projectID := r.PathValue("id")
	var d models.DocPage
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ProjectID = projectID
	if err := s.Store.SaveDocPage(&d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "doc_created", projectID)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateDoc(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : modification de page Wiki non autorisée.")
		return
	}
	docID := r.PathValue("docId")
	var d models.DocPage
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d.ID = docID
	if err := s.Store.SaveDocPage(&d); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "doc_updated", d.ProjectID)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	u := middleware.GetUser(r)
	if !middleware.CanWrite(u) {
		writeErr(w, http.StatusForbidden, "Mode lecture seule : suppression de page Wiki non autorisée.")
		return
	}
	docID := r.PathValue("docId")
	if err := s.Store.DeleteDocPage(docID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcastChange(u.WorkspaceID, "doc_deleted", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": docID})
}
