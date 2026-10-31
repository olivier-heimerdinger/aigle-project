package database

import "github.com/aigle/aigle-server/internal/models"

// SeedIfNeeded initialise les Workspaces SaaS, l'administrateur par défaut et deux projets complets si la base est vide.
func (s *Store) SeedIfNeeded() error {
	var count int
	if err := s.queryRow(`SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// 1. Workspaces (Tenants SaaS)
	workspaces := []models.Workspace{
		{
			ID:          "ws_default",
			Name:        "Studio Heimerdinger (Principal)",
			Slug:        "heimerdinger",
			Description: "Espace de travail principal de l'équipe Studio Heimerdinger.",
			Status:      "active",
			MaxProjects: 100,
			MaxUsers:    100,
		},
		{
			ID:          "ws_client_demo",
			Name:        "Espace Client Externe (Isolé)",
			Slug:        "client-externe",
			Description: "Tenant secondaire démontrant l'isolation multi-organisations du serveur SaaS.",
			Status:      "active",
			MaxProjects: 20,
			MaxUsers:    25,
		},
	}
	for i := range workspaces {
		_ = s.SaveWorkspace(&workspaces[i])
	}

	// 2. Utilisateur SuperAdmin initial
	adminUser := &models.User{
		ID:          "usr_admin",
		Username:    "admin",
		Email:       "admin@heimerdinger.studio",
		DisplayName: "Administrateur Plateforme",
		Role:        models.RoleSuperAdmin,
		Groups:      "admins,superadmin",
		WorkspaceID: "ws_default",
		IsActive:    true,
	}
	_ = s.SaveUser(adminUser)

	// Trace d'audit initiale
	s.AddAuditLog("ws_default", "system@heimerdinger.studio", "superadmin", "SYSTEM_INIT", "workspace", "ws_default", "Initialisation de la base de données SaaS et des espaces de travail", "127.0.0.1")

	// 3. Intervenants de l'équipe avec taux horaires (THM / TJM)
	members := []models.Member{
		{ID: "mem_1", WorkspaceID: "ws_default", Name: "Olivier M.", Email: "olivier@heimerdinger.studio", Role: "Directeur de Projet / Architecte", HourlyRate: 95.0, AvatarColor: "#4f46e5"},
		{ID: "mem_2", WorkspaceID: "ws_default", Name: "Clara V.", Email: "clara@heimerdinger.studio", Role: "Lead Développeur Go", HourlyRate: 75.0, AvatarColor: "#0891b2"},
		{ID: "mem_3", WorkspaceID: "ws_default", Name: "Thomas R.", Email: "thomas@heimerdinger.studio", Role: "Ingénieur Frontend Svelte", HourlyRate: 65.0, AvatarColor: "#059669"},
		{ID: "mem_4", WorkspaceID: "ws_default", Name: "Sarah L.", Email: "sarah@heimerdinger.studio", Role: "DevSecOps & Sécurité Authelia", HourlyRate: 70.0, AvatarColor: "#d97706"},
	}
	for i := range members {
		_ = s.CreateMember(&members[i])
	}

	// 4. Projet 1 : Déploiement Cloud SaaS & Sécurité Authelia
	p1 := &models.Project{
		ID:                "prj_cloud",
		WorkspaceID:       "ws_default",
		Code:              "AIG",
		Name:              "Plateforme SaaS Aigle & Portail SSO Authelia",
		Description:       "Architecture conteneurisée (Docker / Traefik / Authelia / PostgreSQL 16) avec moteur MS Project CPM, Kanban hybride et administration multi-tenant.",
		ClientName:        "Heimerdinger Studio",
		Status:            "active",
		StartDate:         "2026-10-15",
		TargetEndDate:     "2026-12-18",
		InitialBudget:     45000.0,
		DefaultHourlyRate: 75.0,
		Currency:          "EUR",
	}
	_ = s.CreateProject(p1)

	wbsNodes := []*models.WBSNode{
		{ID: "wbs_1", ProjectID: "prj_cloud", ParentID: "", WBSCode: "1", Title: "Phase 1 : Architecture & Sécurité Zero-Trust", NodeType: "phase", StartDate: "2026-10-15", EndDate: "2026-10-29", DurationDays: 15, Progress: 100, AllocatedBudget: 8500, AssignedMemberID: "mem_1", OrderIndex: 1},
		{ID: "wbs_1_1", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.1", Title: "Cadrage Multi-Tenant & Modèle Financier EVM", NodeType: "task", StartDate: "2026-10-15", EndDate: "2026-10-21", DurationDays: 7, Progress: 100, AllocatedBudget: 4000, EstimatedHours: 40, AssignedMemberID: "mem_1", OrderIndex: 2},
		{ID: "wbs_1_2", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.2", Title: "Intégration Traefik + Authelia ForwardAuth", NodeType: "task", StartDate: "2026-10-22", EndDate: "2026-10-28", DurationDays: 7, Progress: 100, AllocatedBudget: 4500, EstimatedHours: 50, AssignedMemberID: "mem_4", OrderIndex: 3},
		{ID: "wbs_1_3", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.3", Title: "Jalon J1 : Architecture Sécurisée Validée", NodeType: "milestone", StartDate: "2026-10-29", EndDate: "2026-10-29", DurationDays: 0, Progress: 100, AllocatedBudget: 0, AssignedMemberID: "mem_1", OrderIndex: 4},

		{ID: "wbs_2", ProjectID: "prj_cloud", ParentID: "", WBSCode: "2", Title: "Phase 2 : Développement Serveur SaaS & Console Admin", NodeType: "phase", StartDate: "2026-10-30", EndDate: "2026-11-24", DurationDays: 26, Progress: 55, AllocatedBudget: 24000, AssignedMemberID: "mem_2", OrderIndex: 5},
		{ID: "wbs_2_1", ProjectID: "prj_cloud", ParentID: "wbs_2", WBSCode: "2.1", Title: "Moteur CPM Gantt, Financier & Isolation PostgreSQL", NodeType: "task", StartDate: "2026-10-30", EndDate: "2026-11-10", DurationDays: 12, Progress: 80, AllocatedBudget: 9500, EstimatedHours: 110, AssignedMemberID: "mem_2", OrderIndex: 6},
		{ID: "wbs_2_2", ProjectID: "prj_cloud", ParentID: "wbs_2", WBSCode: "2.2", Title: "Console d'Administration SaaS & Audit Trail", NodeType: "task", StartDate: "2026-11-11", EndDate: "2026-11-24", DurationDays: 14, Progress: 35, AllocatedBudget: 14500, EstimatedHours: 180, AssignedMemberID: "mem_3", OrderIndex: 7},

		{ID: "wbs_3", ProjectID: "prj_cloud", ParentID: "", WBSCode: "3", Title: "Phase 3 : Conteneurisation Docker & Mise en Production", NodeType: "phase", StartDate: "2026-11-25", EndDate: "2026-12-10", DurationDays: 16, Progress: 0, AllocatedBudget: 9500, AssignedMemberID: "mem_4", OrderIndex: 8},
		{ID: "wbs_3_1", ProjectID: "prj_cloud", ParentID: "wbs_3", WBSCode: "3.1", Title: "Durcissement Conteneur Non-Root & Audit OWASP", NodeType: "task", StartDate: "2026-11-25", EndDate: "2026-12-08", DurationDays: 14, Progress: 0, AllocatedBudget: 9500, EstimatedHours: 120, AssignedMemberID: "mem_4", OrderIndex: 9},
		{ID: "wbs_3_2", ProjectID: "prj_cloud", ParentID: "wbs_3", WBSCode: "3.2", Title: "Jalon J2 : Ouverture aigle.heimerdinger.studio", NodeType: "milestone", StartDate: "2026-12-09", EndDate: "2026-12-09", DurationDays: 0, Progress: 0, AllocatedBudget: 0, AssignedMemberID: "mem_1", OrderIndex: 10},
	}
	for _, n := range wbsNodes {
		_ = s.SaveWBSNode(n)
	}

	deps := []models.WBSDependency{
		{ID: "dep_1", ProjectID: "prj_cloud", PredecessorID: "wbs_1_1", SuccessorID: "wbs_1_2", DepType: "FS", LagDays: 0},
		{ID: "dep_2", ProjectID: "prj_cloud", PredecessorID: "wbs_1_2", SuccessorID: "wbs_1_3", DepType: "FS", LagDays: 0},
		{ID: "dep_3", ProjectID: "prj_cloud", PredecessorID: "wbs_1_3", SuccessorID: "wbs_2_1", DepType: "FS", LagDays: 1},
		{ID: "dep_4", ProjectID: "prj_cloud", PredecessorID: "wbs_2_1", SuccessorID: "wbs_2_2", DepType: "FS", LagDays: 0},
		{ID: "dep_5", ProjectID: "prj_cloud", PredecessorID: "wbs_2_2", SuccessorID: "wbs_3_1", DepType: "FS", LagDays: 0},
		{ID: "dep_6", ProjectID: "prj_cloud", PredecessorID: "wbs_3_1", SuccessorID: "wbs_3_2", DepType: "FS", LagDays: 0},
	}
	for i := range deps {
		_ = s.CreateWBSDependency(&deps[i])
	}

	tickets := []*models.Ticket{
		{
			ID: "tck_1", ProjectID: "prj_cloud", WBSNodeID: "wbs_1_1", TicketKey: "AIG-1",
			Title: "Validation du modèle financier Budget / Consommé / RAF / Atterrissage",
			Description: "Formules EVM et consolidation automatique par phase et par projet.",
			TicketType: "task", Status: "done", Priority: "high", AssigneeID: "mem_1",
			EstimatedHours: 36, LoggedHours: 34, RemainingHours: 0, DueDate: "2026-10-21", Tags: "finance,evm",
		},
		{
			ID: "tck_2", ProjectID: "prj_cloud", WBSNodeID: "wbs_1_2", TicketKey: "AIG-2",
			Title: "Configuration du middleware ForwardAuth Traefik + Authelia (2FA)",
			Description: "Protection de aigle.heimerdinger.studio via airgle-auth.heimerdinger.studio.",
			TicketType: "feature", Status: "done", Priority: "critical", AssigneeID: "mem_4",
			EstimatedHours: 48, LoggedHours: 48, RemainingHours: 0, DueDate: "2026-10-28", Tags: "securite,authelia,traefik",
		},
		{
			ID: "tck_3", ProjectID: "prj_cloud", WBSNodeID: "wbs_2_1", TicketKey: "AIG-3",
			Title: "Moteur du Chemin Critique (CPM) & Isolation Multi-Tenant PostgreSQL",
			Description: "Calcul des marges et cloisonnement strict des requêtes par workspace_id.",
			TicketType: "feature", Status: "in_progress", Priority: "critical", AssigneeID: "mem_2",
			EstimatedHours: 64, LoggedHours: 48, RemainingHours: 16, DueDate: "2026-11-10", Tags: "backend,postgres,cpm",
		},
		{
			ID: "tck_4", ProjectID: "prj_cloud", WBSNodeID: "wbs_2_1", ParentTicketID: "tck_3", TicketKey: "AIG-4",
			Title: "Sous-ticket : Tests anti-injection SQL et validation des rôles RBAC",
			Description: "Sous-ticket rattaché à #AIG-3 pour vérifier l'étanchéité entre tenants.",
			TicketType: "task", Status: "review", Priority: "high", AssigneeID: "mem_4",
			EstimatedHours: 16, LoggedHours: 12, RemainingHours: 4, DueDate: "2026-11-08", Tags: "securite,owasp,sous-ticket",
		},
		{
			ID: "tck_5", ProjectID: "prj_cloud", WBSNodeID: "wbs_2_2", TicketKey: "AIG-5",
			Title: "Développement de la Console d'Administration SaaS (Tenants, Users, Audit)",
			Description: "Interface de gestion des espaces clients, des rôles utilisateurs et du journal d'audit.",
			TicketType: "feature", Status: "in_progress", Priority: "high", AssigneeID: "mem_3",
			EstimatedHours: 80, LoggedHours: 32, RemainingHours: 48, DueDate: "2026-11-20", Tags: "admin,svelte,rbac",
		},
		{
			ID: "tck_6", ProjectID: "prj_cloud", WBSNodeID: "", TicketKey: "AIG-6",
			Title: "Coûts d'infrastructure Cloud & Certificats TLS Wildcard",
			Description: "Ticket transverse rattaché globalement au budget du projet.",
			TicketType: "task", Status: "in_progress", Priority: "medium", AssigneeID: "mem_4",
			EstimatedHours: 12, LoggedHours: 6, RemainingHours: 6, FixedCost: 1250.0, DueDate: "2026-11-15", Tags: "infra,docker,budget-transverse",
		},
	}
	for _, t := range tickets {
		_ = s.SaveTicket(t)
	}

	docs := []*models.DocPage{
		{
			ID: "doc_1", ProjectID: "prj_cloud", Title: "1. Architecture SaaS, Traefik & Sécurité Authelia",
			Slug: "architecture-saas-authelia", Category: "architecture", OrderIndex: 1, LinkedTicketIDs: "AIG-2,AIG-3,AIG-4",
			ContentMD: `# Architecture SaaS & Sécurité Zero-Trust

## 1. Flux d'Authentification (Traefik + Authelia ForwardAuth)
Toute requête vers **aigle.heimerdinger.studio** est interceptée par **Traefik** (sur le réseau externe ` + "`web`" + `) et vérifiée auprès d'**Authelia** (**airgle-auth.heimerdinger.studio**) :
- Si l'utilisateur n'est pas connecté, il est redirigé vers le portail Authelia (avec support 2FA TOTP / WebAuthn).
- Une fois authentifié, Authelia injecte les en-têtes ` + "`Remote-User`" + `, ` + "`Remote-Email`" + `, ` + "`Remote-Name`" + ` et ` + "`Remote-Groups`" + ` vers le conteneur Go **Aigle Server** (Voir **#AIG-2**).

## 2. Isolation Réseau & Base de Données
- Seuls les routeurs HTTP passent par Traefik.
- La base **PostgreSQL 16** et **Redis** résident exclusivement sur le réseau bridge interne isolé ` + "`braigle`" + ` sans aucun port exposé sur l'hôte (Voir **#AIG-3** et **#AIG-4**).`,
		},
	}
	for _, d := range docs {
		_ = s.SaveDocPage(d)
	}

	return nil
}

