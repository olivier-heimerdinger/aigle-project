// Package api expose les endpoints HTTP REST, le flux temps réel Server-Sent Events (SSE)
// pour le travail en équipe, et sert l'application monopage (SPA) Svelte embarquée.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aigle/aigle/internal/config"
	"github.com/aigle/aigle/internal/database"
	"github.com/aigle/aigle/internal/engine"
	"github.com/aigle/aigle/internal/models"
)

// Server encapsule la configuration, l'accès à la base de données, les fichiers statiques Svelte
// et le registre des clients connectés en temps réel via SSE.
type Server struct {
	Cfg       *config.Config
	Store     *database.Store
	Frontend  fs.FS
	clientsMu sync.Mutex
	clients   map[chan string]struct{}
}

// NewServer instancie un nouveau serveur HTTP Aigle.
func NewServer(cfg *config.Config, store *database.Store, frontend fs.FS) *Server {
	return &Server{
		Cfg:      cfg,
		Store:    store,
		Frontend: frontend,
		clients:  make(map[chan string]struct{}),
	}
}

// Handler construit et retourne le routeur HTTP complet (API REST + SSE + SPA Frontend).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Système & Temps Réel ---
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/events", s.handleSSE)

	// --- Intervenants & Grille Tarifaire (THM / TJM) ---
	mux.HandleFunc("GET /api/members", s.handleGetMembers)
	mux.HandleFunc("POST /api/members", s.handleCreateMember)
	mux.HandleFunc("PUT /api/members/{id}", s.handleUpdateMember)

	// --- Portefeuille Multi-Projets & Environnement Projet ---
	mux.HandleFunc("GET /api/projects", s.handleGetProjects)
	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/projects/{id}/full", s.handleGetProjectFull)
	mux.HandleFunc("PUT /api/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)

	// --- Planification MS Project / Gantt (WBS & Dépendances) ---
	mux.HandleFunc("POST /api/projects/{id}/wbs", s.handleCreateWBSNode)
	mux.HandleFunc("PUT /api/wbs/{nodeId}", s.handleUpdateWBSNode)
	mux.HandleFunc("DELETE /api/wbs/{nodeId}", s.handleDeleteWBSNode)
	mux.HandleFunc("POST /api/projects/{id}/dependencies", s.handleCreateDependency)
	mux.HandleFunc("DELETE /api/dependencies/{depId}", s.handleDeleteDependency)

	// --- Kanban, Tickets Hybrides & Pointages / RAF ---
	mux.HandleFunc("POST /api/projects/{id}/tickets", s.handleCreateTicket)
	mux.HandleFunc("PUT /api/tickets/{ticketId}", s.handleUpdateTicket)
	mux.HandleFunc("PATCH /api/tickets/{ticketId}/move", s.handleMoveTicket)
	mux.HandleFunc("DELETE /api/tickets/{ticketId}", s.handleDeleteTicket)
	mux.HandleFunc("POST /api/tickets/{ticketId}/timelogs", s.handleAddTimeLog)

	// --- Gestion Documentaire (Wiki Projet) ---
	mux.HandleFunc("POST /api/projects/{id}/docs", s.handleCreateDoc)
	mux.HandleFunc("PUT /api/docs/{docId}", s.handleUpdateDoc)
	mux.HandleFunc("DELETE /api/docs/{docId}", s.handleDeleteDoc)

	// --- Service de l'interface Svelte embarquée (SPA Fallback) ---
	if s.Frontend != nil {
		fileServer := http.FileServer(http.FS(s.Frontend))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if _, err := fs.Stat(s.Frontend, path); err != nil {
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	return corsMiddleware(mux)
}

// corsMiddleware autorise les requêtes Cross-Origin (notamment utile en mode dev Vite :5173 -> :8080).
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Broadcast diffuse un événement temps réel à tous les navigateurs connectés via SSE.
func (s *Server) Broadcast(eventType, projectID string) {
	msg := fmt.Sprintf(`{"type":%q,"project_id":%q,"ts":%d}`, eventType, projectID, time.Now().UnixMilli())
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// handleSSE maintient une connexion Server-Sent Events ouverte pour synchroniser les vues en équipe.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE non supporté", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 8)
	s.clientsMu.Lock()
	s.clients[ch] = struct{}{}
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, ch)
		s.clientsMu.Unlock()
	}()

	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"app":       "Aigle",
		"mode":      s.Cfg.Mode,
		"db_driver": s.Store.Driver,
	})
}

// --- Handlers Intervenants (Members) ---

func (s *Server) handleGetMembers(w http.ResponseWriter, r *http.Request) {
	members, err := s.Store.GetMembers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (s *Server) handleCreateMember(w http.ResponseWriter, r *http.Request) {
	var m models.Member
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	if m.Name == "" {
		writeError(w, http.StatusBadRequest, "Le nom est obligatoire")
		return
	}
	if err := s.Store.CreateMember(&m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("member_updated", "")
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var m models.Member
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	m.ID = id
	if err := s.Store.UpdateMember(&m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("member_updated", "")
	writeJSON(w, http.StatusOK, m)
}

// --- Handlers Projets & Consolidation Globale ---

func (s *Server) handleGetProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.Store.GetProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	members, _ := s.Store.GetMembers()

	// Calcule à la volée la synthèse financière de chaque projet pour la vue Portefeuille
	for _, p := range projects {
		nodes, _ := s.Store.GetWBSNodes(p.ID)
		tickets, _ := s.Store.GetTickets(p.ID)
		logs, _ := s.Store.GetTimeLogsByProject(p.ID)
		engine.ComputeProjectFinancials(p, nodes, tickets, members, logs)
	}

	writeJSON(w, http.StatusOK, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	if p.Name == "" || p.Code == "" {
		writeError(w, http.StatusBadRequest, "Le nom et le code projet sont requis")
		return
	}
	if p.StartDate == "" {
		p.StartDate = time.Now().Format("2006-01-02")
	}
	if p.TargetEndDate == "" {
		p.TargetEndDate = time.Now().AddDate(0, 2, 0).Format("2006-01-02")
	}
	if err := s.Store.CreateProject(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Crée automatiquement une première Phase dans le Gantt pour faciliter la prise en main
	defaultPhase := &models.WBSNode{
		ProjectID:       p.ID,
		WBSCode:         "1",
		Title:           "Phase 1 : Initialisation & Cadrage",
		NodeType:        "phase",
		StartDate:       p.StartDate,
		EndDate:         p.TargetEndDate,
		DurationDays:    10,
		AllocatedBudget: p.InitialBudget * 0.25,
		OrderIndex:      1,
	}
	_ = s.Store.SaveWBSNode(defaultPhase)

	s.Broadcast("project_created", p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	p.ID = id
	if err := s.Store.UpdateProject(&p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("project_updated", p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteProject(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("project_deleted", id)
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// recalculateAndPersistSchedule charge l'arbre WBS et ses dépendances, exécute le moteur CPM et sauvegarde les dates recalées.
func (s *Server) recalculateAndPersistSchedule(projectID string) ([]*models.WBSNode, []models.WBSDependency, error) {
	nodes, err := s.Store.GetWBSNodes(projectID)
	if err != nil {
		return nil, nil, err
	}
	deps, err := s.Store.GetWBSDependencies(projectID)
	if err != nil {
		return nil, nil, err
	}
	if err := engine.ComputeSchedule(nodes, deps); err != nil {
		return nodes, deps, err
	}
	for _, n := range nodes {
		_ = s.Store.SaveWBSNode(n)
	}
	return nodes, deps, nil
}

// handleGetProjectFull retourne l'intégralité de l'environnement d'un projet en un seul appel JSON.
func (s *Server) handleGetProjectFull(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	project, err := s.Store.GetProject(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Projet introuvable")
		return
	}

	nodes, deps, _ := s.recalculateAndPersistSchedule(projectID)
	tickets, _ := s.Store.GetTickets(projectID)
	logsByTicket, _ := s.Store.GetTimeLogsByProject(projectID)
	members, _ := s.Store.GetMembers()
	docs, _ := s.Store.GetDocPages(projectID)

	summary := engine.ComputeProjectFinancials(project, nodes, tickets, members, logsByTicket)

	writeJSON(w, http.StatusOK, map[string]any{
		"project":      project,
		"financials":   summary,
		"wbs_nodes":    nodes,
		"dependencies": deps,
		"tickets":      tickets,
		"time_logs":    logsByTicket,
		"docs":         docs,
		"members":      members,
	})
}

// --- Handlers WBS / Gantt ---

func (s *Server) handleCreateWBSNode(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var n models.WBSNode
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	n.ProjectID = projectID
	if n.StartDate == "" {
		n.StartDate = time.Now().Format("2006-01-02")
	}
	if n.DurationDays <= 0 && n.NodeType != "milestone" {
		n.DurationDays = 5
	}
	if n.EndDate == "" {
		start := engine.ParseDate(n.StartDate)
		if n.NodeType == "milestone" {
			n.EndDate = n.StartDate
		} else {
			n.EndDate = engine.FormatDate(start.AddDate(0, 0, n.DurationDays-1))
		}
	}
	if err := s.Store.SaveWBSNode(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _, _ = s.recalculateAndPersistSchedule(projectID)
	s.Broadcast("wbs_updated", projectID)
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateWBSNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("nodeId")
	var n models.WBSNode
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	n.ID = nodeID
	if n.DurationDays > 0 && n.NodeType != "milestone" && n.StartDate != "" {
		start := engine.ParseDate(n.StartDate)
		n.EndDate = engine.FormatDate(start.AddDate(0, 0, n.DurationDays-1))
	}
	if err := s.Store.SaveWBSNode(&n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _, _ = s.recalculateAndPersistSchedule(n.ProjectID)
	s.Broadcast("wbs_updated", n.ProjectID)
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteWBSNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("nodeId")
	if err := s.Store.DeleteWBSNode(nodeID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("wbs_updated", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": nodeID})
}

func (s *Server) handleCreateDependency(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var d models.WBSDependency
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	d.ProjectID = projectID
	if d.PredecessorID == "" || d.SuccessorID == "" || d.PredecessorID == d.SuccessorID {
		writeError(w, http.StatusBadRequest, "Prédécesseur et successeur invalides")
		return
	}

	// Vérifie l'absence de cycle avant d'enregistrer la dépendance
	nodes, _ := s.Store.GetWBSNodes(projectID)
	deps, _ := s.Store.GetWBSDependencies(projectID)
	testDeps := append(deps, d)
	if err := engine.ComputeSchedule(nodes, testDeps); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.Store.CreateWBSDependency(&d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _, _ = s.recalculateAndPersistSchedule(projectID)
	s.Broadcast("wbs_updated", projectID)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleDeleteDependency(w http.ResponseWriter, r *http.Request) {
	depID := r.PathValue("depId")
	if err := s.Store.DeleteWBSDependency(depID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("wbs_updated", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": depID})
}

// --- Handlers Tickets, Kanban & Pointages (TimeLogs / RAF) ---

func (s *Server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var t models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	t.ProjectID = projectID
	if t.Status == "" {
		t.Status = "todo"
	}
	if t.TicketType == "" {
		t.TicketType = "task"
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	// Initialise par défaut le Reste à faire (RAF) à la charge initiale estimée
	if t.RemainingHours == 0 && t.EstimatedHours > 0 && t.Status != "done" {
		t.RemainingHours = t.EstimatedHours
	}
	if err := s.Store.SaveTicket(&t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("ticket_updated", projectID)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketId")
	var t models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	t.ID = ticketID
	if t.Status == "done" {
		t.RemainingHours = 0
	}
	if err := s.Store.SaveTicket(&t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("ticket_updated", t.ProjectID)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleMoveTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketId")
	var payload struct {
		Status    string  `json:"status"`
		WBSNodeID *string `json:"wbs_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	t, err := s.Store.GetTicket(ticketID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Ticket introuvable")
		return
	}
	if payload.Status != "" {
		t.Status = payload.Status
		if t.Status == "done" {
			t.RemainingHours = 0
		}
	}
	if payload.WBSNodeID != nil {
		t.WBSNodeID = *payload.WBSNodeID
	}
	if err := s.Store.SaveTicket(t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("ticket_updated", t.ProjectID)
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketId")
	if err := s.Store.DeleteTicket(ticketID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("ticket_updated", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": ticketID})
}

func (s *Server) handleAddTimeLog(w http.ResponseWriter, r *http.Request) {
	ticketID := r.PathValue("ticketId")
	var payload struct {
		MemberID       string   `json:"member_id"`
		Hours          float64  `json:"hours"`
		WorkDate       string   `json:"work_date"`
		Comment        string   `json:"comment"`
		RemainingHours *float64 `json:"remaining_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	if payload.Hours <= 0 {
		writeError(w, http.StatusBadRequest, "Le nombre d'heures doit être supérieur à 0")
		return
	}
	logEntry := &models.TimeLog{
		TicketID: ticketID,
		MemberID: payload.MemberID,
		Hours:    payload.Hours,
		WorkDate: payload.WorkDate,
		Comment:  payload.Comment,
	}
	if err := s.Store.AddTimeLog(logEntry, payload.RemainingHours); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("ticket_updated", "")
	writeJSON(w, http.StatusCreated, logEntry)
}

// --- Handlers Documentation (Wiki) ---

func (s *Server) handleCreateDoc(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var d models.DocPage
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	d.ProjectID = projectID
	if err := s.Store.SaveDocPage(&d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("doc_updated", projectID)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleUpdateDoc(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docId")
	var d models.DocPage
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "JSON invalide")
		return
	}
	d.ID = docID
	if err := s.Store.SaveDocPage(&d); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("doc_updated", d.ProjectID)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDoc(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("docId")
	if err := s.Store.DeleteDocPage(docID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Broadcast("doc_updated", "")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": docID})
}
