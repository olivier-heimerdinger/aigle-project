package database

import "github.com/aigle/aigle/internal/models"

// SeedIfNeeded populates the database with a rich demonstration portfolio if no projects exist.
func (s *Store) SeedIfNeeded() error {
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// 1. Default Workspace
	_, _ = s.DB.Exec(`INSERT INTO workspaces (id, name, slug, mode) VALUES ('ws_default', 'Espace Aigle Principal', 'default', 'local')`)

	// 2. Team Members with realistic TJM / Hourly Rates
	members := []models.Member{
		{ID: "mem_1", WorkspaceID: "ws_default", Name: "Olivier M.", Email: "olivier@aigle.local", Role: "Directeur de Projet / Architecte", HourlyRate: 95.0, AvatarColor: "#4f46e5"},
		{ID: "mem_2", WorkspaceID: "ws_default", Name: "Clara V.", Email: "clara@aigle.local", Role: "Lead Développeur Go", HourlyRate: 75.0, AvatarColor: "#0891b2"},
		{ID: "mem_3", WorkspaceID: "ws_default", Name: "Thomas R.", Email: "thomas@aigle.local", Role: "Ingénieur Frontend Svelte", HourlyRate: 65.0, AvatarColor: "#059669"},
		{ID: "mem_4", WorkspaceID: "ws_default", Name: "Sarah L.", Email: "sarah@aigle.local", Role: "QA & DevSecOps", HourlyRate: 70.0, AvatarColor: "#d97706"},
	}
	for i := range members {
		_ = s.CreateMember(&members[i])
	}

	// 3. Project 1: Refonte Plateforme Cloud & SaaS
	p1 := &models.Project{
		ID:                "prj_cloud",
		WorkspaceID:       "ws_default",
		Code:              "AIG",
		Name:              "Plateforme SaaS & Architecture Hybride",
		Description:       "Développement de la plateforme cœur en Go + Svelte avec gestion financière (Budget, Consommé, RAF, Atterrissage) et planification MS Project.",
		ClientName:        "Direction Produit Interne",
		Status:            "active",
		StartDate:         "2026-10-15",
		TargetEndDate:     "2026-12-18",
		InitialBudget:     45000.0,
		DefaultHourlyRate: 75.0,
		Currency:          "EUR",
	}
	_ = s.CreateProject(p1)

	// WBS / Gantt structure for Project 1
	wbsNodes := []*models.WBSNode{
		// Phase 1
		{ID: "wbs_1", ProjectID: "prj_cloud", ParentID: "", WBSCode: "1", Title: "Phase 1 : Cadrage & Architecture", NodeType: "phase", StartDate: "2026-10-15", EndDate: "2026-10-29", DurationDays: 15, Progress: 100, AllocatedBudget: 8500, AssignedMemberID: "mem_1", OrderIndex: 1},
		{ID: "wbs_1_1", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.1", Title: "Spécifications fonctionnelles & Modèle financier", NodeType: "task", StartDate: "2026-10-15", EndDate: "2026-10-21", DurationDays: 7, Progress: 100, AllocatedBudget: 4000, EstimatedHours: 40, AssignedMemberID: "mem_1", OrderIndex: 2},
		{ID: "wbs_1_2", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.2", Title: "Architecture Base de données (SQLite / Postgres)", NodeType: "task", StartDate: "2026-10-22", EndDate: "2026-10-28", DurationDays: 7, Progress: 100, AllocatedBudget: 4500, EstimatedHours: 50, AssignedMemberID: "mem_2", OrderIndex: 3},
		{ID: "wbs_1_3", ProjectID: "prj_cloud", ParentID: "wbs_1", WBSCode: "1.3", Title: "Jalon J1 : Architecture Validée", NodeType: "milestone", StartDate: "2026-10-29", EndDate: "2026-10-29", DurationDays: 0, Progress: 100, AllocatedBudget: 0, AssignedMemberID: "mem_1", OrderIndex: 4},

		// Phase 2
		{ID: "wbs_2", ProjectID: "prj_cloud", ParentID: "", WBSCode: "2", Title: "Phase 2 : Développement Cœur & Moteurs", NodeType: "phase", StartDate: "2026-10-30", EndDate: "2026-11-24", DurationDays: 26, Progress: 45, AllocatedBudget: 24000, AssignedMemberID: "mem_2", OrderIndex: 5},
		{ID: "wbs_2_1", ProjectID: "prj_cloud", ParentID: "wbs_2", WBSCode: "2.1", Title: "Moteur CPM Gantt & Calcul Financier Go", NodeType: "task", StartDate: "2026-10-30", EndDate: "2026-11-10", DurationDays: 12, Progress: 70, AllocatedBudget: 9500, EstimatedHours: 110, AssignedMemberID: "mem_2", OrderIndex: 6},
		{ID: "wbs_2_2", ProjectID: "prj_cloud", ParentID: "wbs_2", WBSCode: "2.2", Title: "Interface Svelte (Gantt, Kanban & Finances)", NodeType: "task", StartDate: "2026-11-11", EndDate: "2026-11-24", DurationDays: 14, Progress: 25, AllocatedBudget: 14500, EstimatedHours: 180, AssignedMemberID: "mem_3", OrderIndex: 7},

		// Phase 3
		{ID: "wbs_3", ProjectID: "prj_cloud", ParentID: "", WBSCode: "3", Title: "Phase 3 : Qualification & Déploiement SaaS", NodeType: "phase", StartDate: "2026-11-25", EndDate: "2026-12-10", DurationDays: 16, Progress: 0, AllocatedBudget: 9500, AssignedMemberID: "mem_4", OrderIndex: 8},
		{ID: "wbs_3_1", ProjectID: "prj_cloud", ParentID: "wbs_3", WBSCode: "3.1", Title: "Tests de charge, Sécurité & Recette", NodeType: "task", StartDate: "2026-11-25", EndDate: "2026-12-08", DurationDays: 14, Progress: 0, AllocatedBudget: 9500, EstimatedHours: 120, AssignedMemberID: "mem_4", OrderIndex: 9},
		{ID: "wbs_3_2", ProjectID: "prj_cloud", ParentID: "wbs_3", WBSCode: "3.2", Title: "Jalon J2 : Mise en Production v1.0", NodeType: "milestone", StartDate: "2026-12-09", EndDate: "2026-12-09", DurationDays: 0, Progress: 0, AllocatedBudget: 0, AssignedMemberID: "mem_1", OrderIndex: 10},
	}
	for _, n := range wbsNodes {
		_ = s.SaveWBSNode(n)
	}

	// Dependencies for Project 1
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

	// Tickets for Project 1 (Both Pinned to Gantt and Unpinned/Global Project tickets)
	tickets := []*models.Ticket{
		{
			ID:             "tck_1",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_1_1",
			TicketKey:      "AIG-1",
			Title:          "Définir les règles de calcul Budget / Consommé / RAF / Atterrissage",
			Description:    "Valider les formules d'atterrissage financier (EAC = AC + ETC) et la remontée des tickets épinglés vers les lots du Gantt.",
			TicketType:     "task",
			Status:         "done",
			Priority:       "high",
			AssigneeID:     "mem_1",
			EstimatedHours: 36,
			LoggedHours:    34,
			RemainingHours: 0,
			FixedCost:      0,
			DueDate:        "2026-10-21",
			Tags:           "finance,spécification",
		},
		{
			ID:             "tck_2",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_1_2",
			TicketKey:      "AIG-2",
			Title:          "Implémenter le couche SQLite WAL & Abstraction Postgres SaaS",
			Description:    "Mise en place de modernc.org/sqlite sans CGO et gestion des migrations automatiques.",
			TicketType:     "feature",
			Status:         "done",
			Priority:       "high",
			AssigneeID:     "mem_2",
			EstimatedHours: 48,
			LoggedHours:    52,
			RemainingHours: 0,
			FixedCost:      0,
			DueDate:        "2026-10-28",
			Tags:           "backend,database,saas",
		},
		{
			ID:             "tck_3",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_2_1",
			TicketKey:      "AIG-3",
			Title:          "Algorithme du Chemin Critique (CPM) et recalcul auto des dépendances FS/SS",
			Description:    "Calculer EarlyStart, EarlyFinish, LateStart, LateFinish et la marge totale sur le graphe de tâches.",
			TicketType:     "feature",
			Status:         "in_progress",
			Priority:       "critical",
			AssigneeID:     "mem_2",
			EstimatedHours: 64,
			LoggedHours:    48,
			RemainingHours: 20, // Léger dépassement prévu (Atterrissage 68h vs 64h)
			FixedCost:      0,
			DueDate:        "2026-11-10",
			Tags:           "gantt,moteur,go",
		},
		{
			ID:             "tck_4",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_2_1",
			ParentTicketID: "tck_3", // Sous-ticket enfant de AIG-3 !
			TicketKey:      "AIG-4",
			Title:          "Corriger le calcul des jalons à durée zéro dans la passe arrière",
			Description:    "Sous-ticket rattaché à AIG-3 : un jalon (duration = 0) ne doit pas décaler d'un jour supplémentaire son successeur immédiat.",
			TicketType:     "bug",
			Status:         "review",
			Priority:       "high",
			AssigneeID:     "mem_2",
			EstimatedHours: 16,
			LoggedHours:    12,
			RemainingHours: 4,
			FixedCost:      0,
			DueDate:        "2026-11-08",
			Tags:           "bug,gantt,sous-ticket",
		},
		{
			ID:             "tck_5",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_2_2",
			TicketKey:      "AIG-5",
			Title:          "Composant Gantt interactif SVG + Table WBS synchronisée",
			Description:    "Créer la vue MS Project avec hiérarchie WBS à gauche, barres et flèches de dépendances à droite.",
			TicketType:     "feature",
			Status:         "in_progress",
			Priority:       "high",
			AssigneeID:     "mem_3",
			EstimatedHours: 80,
			LoggedHours:    28,
			RemainingHours: 52,
			FixedCost:      0,
			DueDate:        "2026-11-20",
			Tags:           "svelte,ui,gantt",
		},
		{
			ID:             "tck_6",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "wbs_2_2",
			TicketKey:      "AIG-6",
			Title:          "Tableau Kanban Drag & Drop avec filtres par Phase et alertes RAF",
			Description:    "Permettre le glisser-déposer des tickets et l'ajustement rapide du temps pointé et du RAF.",
			TicketType:     "feature",
			Status:         "todo",
			Priority:       "medium",
			AssigneeID:     "mem_3",
			EstimatedHours: 56,
			LoggedHours:    0,
			RemainingHours: 56,
			FixedCost:      0,
			DueDate:        "2026-11-24",
			Tags:           "svelte,kanban",
		},
		{
			// UNPINNED TICKET (Attached to Project globally, not pinned to any specific Gantt line)
			ID:             "tck_7",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "",
			TicketKey:      "AIG-7",
			Title:          "Risque : Latence d'affichage sur les projets de plus de 2 000 tâches",
			Description:    "Ticket transverse rattaché globalement au projet : prévoir la virtualisation du rendu DOM si nécessaire.",
			TicketType:     "risk",
			Status:         "backlog",
			Priority:       "medium",
			AssigneeID:     "mem_1",
			EstimatedHours: 16,
			LoggedHours:    4,
			RemainingHours: 12,
			FixedCost:      0,
			DueDate:        "2026-11-30",
			Tags:           "risque,performance",
		},
		{
			// UNPINNED TICKET with Fixed Cost (Servers / Licenses)
			ID:             "tck_8",
			ProjectID:      "prj_cloud",
			WBSNodeID:      "",
			TicketKey:      "AIG-8",
			Title:          "Provisionnement Infrastructure Cloud & Certificats Code Signing",
			Description:    "Achat des certificats de signature Windows et environnement de staging PostgreSQL.",
			TicketType:     "task",
			Status:         "in_progress",
			Priority:       "medium",
			AssigneeID:     "mem_4",
			EstimatedHours: 12,
			LoggedHours:    6,
			RemainingHours: 6,
			FixedCost:      1250.0, // Coût fixe matériel/licence
			DueDate:        "2026-11-15",
			Tags:           "devops,infra,budget-transverse",
		},
	}
	for _, t := range tickets {
		_ = s.SaveTicket(t)
	}

	// Seed matching TimeLogs so historical pointages are visible
	logs := []struct {
		TicketID string
		MemberID string
		Hours    float64
		Rate     float64
		Date     string
		Comment  string
	}{
		{"tck_1", "mem_1", 20, 95, "2026-10-18", "Ateliers de cadrage financier et maquettage EVM"},
		{"tck_1", "mem_1", 14, 95, "2026-10-20", "Validation finale des formules d'atterrissage"},
		{"tck_2", "mem_2", 32, 75, "2026-10-25", "Schéma SQL et migrations automatiques"},
		{"tck_2", "mem_2", 20, 75, "2026-10-28", "Tests d'intégration SQLite WAL et repository"},
		{"tck_3", "mem_2", 30, 75, "2026-11-02", "Implémentation du tri topologique et Forward Pass"},
		{"tck_3", "mem_2", 18, 75, "2026-11-05", "Backward Pass et calcul de la marge totale"},
		{"tck_4", "mem_2", 12, 75, "2026-11-06", "Correctif gestion des jalons + tests unitaires"},
		{"tck_5", "mem_3", 28, 65, "2026-11-07", "Grille WBS et rendu SVG des barres et dépendances"},
		{"tck_7", "mem_1", 4, 95, "2026-11-04", "Benchmark initial de performance sur 1 500 nœuds"},
		{"tck_8", "mem_4", 6, 70, "2026-11-05", "Configuration cluster PostgreSQL de pré-production"},
	}
	for _, l := range logs {
		_, _ = s.DB.Exec(`
			INSERT INTO time_logs (id, ticket_id, member_id, hours, hourly_rate_snapshot, work_date, comment)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			NewID("log"), l.TicketID, l.MemberID, l.Hours, l.Rate, l.Date, l.Comment,
		)
	}

	// Seed Documentation / Wiki pages for Project 1
	docs := []*models.DocPage{
		{
			ID:        "doc_1",
			ProjectID: "prj_cloud",
			ParentID:  "",
			Title:     "1. Vision & Architecture Hybride (Local / SaaS)",
			Slug:      "vision-architecture",
			Category:  "architecture",
			OrderIndex: 1,
			LinkedTicketIDs: "AIG-1,AIG-2",
			ContentMD: `# Architecture Hybride d'Aigle

## Objectif
Permettre une exécution **100% autonome en local** (un seul binaire Go embarquant SQLite et le frontend Svelte) tout en permettant un déploiement **en équipe ou en SaaS** connecté à PostgreSQL.

## Articulation Projet ↔ Gantt ↔ Tickets ↔ Budget
- **Rattachement Projet (Obligatoire) :** Chaque ticket (Tâche, Bug, Évolution, Risque, Question) appartient à un projet et alimente son budget global.
- **Épinglage Timeline Gantt (Au cas par cas) :** Un ticket peut être épinglé sur une phase ou tâche du diagramme de Gantt (ex: **#AIG-3** sur la phase *2.1 Moteur CPM*) ou rester transverse au projet (ex: **#AIG-8** pour l'infrastructure).

## Formules Financières
1. **Consommé** = $\sum (\text{Heures pointées} \times \text{Taux horaire}) + \text{Coûts fixes}$
2. **Reste à faire (RAF)** = $\text{Heures restantes estimées} \times \text{Taux horaire}$
3. **Atterrissage prévu** = $\text{Consommé} + \text{RAF}$
4. **Écart à terminaison** = $\text{Budget Initial} - \text{Atterrissage}$`,
		},
		{
			ID:        "doc_2",
			ProjectID: "prj_cloud",
			ParentID:  "",
			Title:     "2. Guide de Pilotage MS Project & Chemin Critique",
			Slug:      "guide-gantt-cpm",
			Category:  "guide",
			OrderIndex: 2,
			LinkedTicketIDs: "AIG-3,AIG-4,AIG-5",
			ContentMD: `# Pilotage de la Timeline (Gantt & WBS)

## Types de liens supportés
- **FS (Fin-à-Début) :** La tâche successeur démarre dès que le prédécesseur se termine (avec décalage optionnel en jours).
- **SS (Début-à-Début) :** Les deux tâches démarrent en parallèle.
- **FF (Fin-à-Fin) :** Les deux tâches doivent se terminer simultanément.

## Chemin Critique
Les tâches affichées en **rouge** sur le diagramme de Gantt ont une marge totale nulle (` + "`TotalSlackDays == 0`" + `). Tout retard sur ces tâches décale directement la date de fin du projet (**Jalon J2**).`,
		},
		{
			ID:        "doc_3",
			ProjectID: "prj_cloud",
			ParentID:  "",
			Title:     "3. Registre des Risques & Décisions (ADR)",
			Slug:      "registre-risques",
			Category:  "risk",
			OrderIndex: 3,
			LinkedTicketIDs: "AIG-7,AIG-8",
			ContentMD: `# Registre des Risques & Décisions Techniques

## Décisions actées (ADR)
- **ADR-001 :** Adoption de ` + "`modernc.org/sqlite`" + ` pour compiler sur Windows et Linux sans dépendance C (` + "`CGO_ENABLED=0`" + `).
- **ADR-002 :** Utilisation de Svelte + TailwindCSS compilé via Bun pour un bundle ultra-léger.

## Suivi des Risques
- Voir le ticket de risque **#AIG-7** concernant la volumétrie d'affichage au-delà de 2 000 lignes WBS.`,
		},
	}
	for _, d := range docs {
		_ = s.SaveDocPage(d)
	}

	// 4. Second Project for Multi-Project Portfolio Switching
	p2 := &models.Project{
		ID:                "prj_erp",
		WorkspaceID:       "ws_default",
		Code:              "ERP",
		Name:              "Migration ERP & Portail Facturation Client",
		Description:       "Projet client externe : interfaçage API de facturation, migration des données historiques et portail client.",
		ClientName:        "Groupe Industriel Horizon",
		Status:            "active",
		StartDate:         "2026-11-01",
		TargetEndDate:     "2027-01-30",
		InitialBudget:     32000.0,
		DefaultHourlyRate: 80.0,
		Currency:          "EUR",
	}
	_ = s.CreateProject(p2)

	erpWBS := []*models.WBSNode{
		{ID: "wbs_erp_1", ProjectID: "prj_erp", ParentID: "", WBSCode: "1", Title: "Lot 1 : Audit & Extraction des données", NodeType: "phase", StartDate: "2026-11-01", EndDate: "2026-11-15", DurationDays: 15, Progress: 60, AllocatedBudget: 12000, AssignedMemberID: "mem_1", OrderIndex: 1},
		{ID: "wbs_erp_1_1", ProjectID: "prj_erp", ParentID: "wbs_erp_1", WBSCode: "1.1", Title: "Connecteur API Comptabilité", NodeType: "task", StartDate: "2026-11-01", EndDate: "2026-11-10", DurationDays: 10, Progress: 80, AllocatedBudget: 7500, EstimatedHours: 80, AssignedMemberID: "mem_2", OrderIndex: 2},
		{ID: "wbs_erp_1_2", ProjectID: "prj_erp", ParentID: "wbs_erp_1", WBSCode: "1.2", Title: "Reprise de l'historique 2023-2026", NodeType: "task", StartDate: "2026-11-11", EndDate: "2026-11-18", DurationDays: 8, Progress: 20, AllocatedBudget: 4500, EstimatedHours: 50, AssignedMemberID: "mem_4", OrderIndex: 3},
		{ID: "wbs_erp_2", ProjectID: "prj_erp", ParentID: "", WBSCode: "2", Title: "Lot 2 : Portail Web Client", NodeType: "phase", StartDate: "2026-11-19", EndDate: "2026-12-15", DurationDays: 27, Progress: 0, AllocatedBudget: 20000, AssignedMemberID: "mem_3", OrderIndex: 4},
	}
	for _, n := range erpWBS {
		_ = s.SaveWBSNode(n)
	}
	_ = s.CreateWBSDependency(&models.WBSDependency{ID: "dep_erp_1", ProjectID: "prj_erp", PredecessorID: "wbs_erp_1_1", SuccessorID: "wbs_erp_1_2", DepType: "FS", LagDays: 0})
	_ = s.CreateWBSDependency(&models.WBSDependency{ID: "dep_erp_2", ProjectID: "prj_erp", PredecessorID: "wbs_erp_1_2", SuccessorID: "wbs_erp_2", DepType: "FS", LagDays: 0})

	erpTickets := []*models.Ticket{
		{
			ID:             "tck_erp_1",
			ProjectID:      "prj_erp",
			WBSNodeID:      "wbs_erp_1_1",
			TicketKey:      "ERP-1",
			Title:          "Développement du connecteur OAuth2 & Synchronisation Factures",
			Description:    "Synchronisation bidirectionnelle des écritures clients.",
			TicketType:     "feature",
			Status:         "in_progress",
			Priority:       "high",
			AssigneeID:     "mem_2",
			EstimatedHours: 60,
			LoggedHours:    45,
			RemainingHours: 15,
			DueDate:        "2026-11-10",
			Tags:           "api,erp",
		},
		{
			ID:             "tck_erp_2",
			ProjectID:      "prj_erp",
			WBSNodeID:      "",
			TicketKey:      "ERP-2",
			Title:          "Question : Format d'export analytique souhaité par la DAF ?",
			Description:    "Point en attente de retour du directeur financier client.",
			TicketType:     "question",
			Status:         "todo",
			Priority:       "medium",
			AssigneeID:     "mem_1",
			EstimatedHours: 8,
			LoggedHours:    2,
			RemainingHours: 6,
			DueDate:        "2026-11-12",
			Tags:           "client,compta",
		},
	}
	for _, t := range erpTickets {
		_ = s.SaveTicket(t)
	}

	return nil
}

