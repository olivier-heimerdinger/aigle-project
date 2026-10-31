package models

import "time"

// Workspace isolates projects and teams (Default workspace in Local mode, multi-tenant in SaaS mode).
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Mode      string    `json:"mode"` // "local" or "saas"
	CreatedAt time.Time `json:"created_at"`
}

// Member represents a user/resource in a workspace with a financial cost rate.
type Member struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`        // e.g., "Chef de Projet", "Lead Dev", "Architecte", "QA"
	HourlyRate  float64   `json:"hourly_rate"` // Taux horaire (THM) ou TJM / 8h
	DailyRate   float64   `json:"daily_rate"`  // TJM indicatif (HourlyRate * 8)
	AvatarColor string    `json:"avatar_color"`
	CreatedAt   time.Time `json:"created_at"`
}

// Project represents a master project with its timeline and financial environment.
type Project struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id"`
	Code              string    `json:"code"`                // e.g., "AIG", "ERP"
	Name              string    `json:"name"`                // Nom du projet
	Description       string    `json:"description"`         // Contexte / Objectifs
	ClientName        string    `json:"client_name"`         // Client ou Département
	Status            string    `json:"status"`              // "planning", "active", "on_hold", "completed"
	StartDate         string    `json:"start_date"`          // YYYY-MM-DD
	TargetEndDate     string    `json:"target_end_date"`     // YYYY-MM-DD
	InitialBudget     float64   `json:"initial_budget"`      // Budget initial global vendu/alloué (€)
	DefaultHourlyRate float64   `json:"default_hourly_rate"` // Taux horaire par défaut si aucun membre assigné
	Currency          string    `json:"currency"`            // "EUR", "USD", etc.
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Computed financial & progress summary (populated dynamically by Engine)
	Financials *FinancialSummary `json:"financials,omitempty"`
}

// WBSNode represents a line in the MS Project / Gantt timeline (Phase, Lot, Task, or Milestone).
type WBSNode struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"project_id"`
	ParentID         string  `json:"parent_id"`          // Empty if root node
	WBSCode          string  `json:"wbs_code"`           // e.g., "1", "1.1", "1.2", "2"
	Title            string  `json:"title"`              // Intitulé de la phase / tâche / jalon
	NodeType         string  `json:"node_type"`          // "phase", "task", "milestone"
	StartDate        string  `json:"start_date"`         // YYYY-MM-DD
	EndDate          string  `json:"end_date"`           // YYYY-MM-DD
	DurationDays     int     `json:"duration_days"`      // Durée en jours ouvrés/calendaires
	Progress         float64 `json:"progress"`           // 0..100 (%)
	AllocatedBudget  float64 `json:"allocated_budget"`   // Budget initial alloué à cette phase/ligne (€)
	EstimatedHours   float64 `json:"estimated_hours"`    // Charge initiale prévue sur la tâche Gantt (h)
	AssignedMemberID string  `json:"assigned_member_id"` // Responsable de la ligne Gantt
	OrderIndex       int     `json:"order_index"`        // Ordre d'affichage

	// Computed fields by CPM & Financial Engines
	IsCritical       bool              `json:"is_critical"`        // Sur le chemin critique (marge = 0)
	TotalSlackDays   int               `json:"total_slack_days"`   // Marge totale en jours
	EarlyStart       string            `json:"early_start"`        // Date au plus tôt
	EarlyFinish      string            `json:"early_finish"`       // Fin au plus tôt
	LateStart        string            `json:"late_start"`         // Date au plus tard
	LateFinish       string            `json:"late_finish"`        // Fin au plus tard
	PinnedTicketCnt  int               `json:"pinned_ticket_cnt"`  // Nombre de tickets épinglés
	DoneTicketCnt    int               `json:"done_ticket_cnt"`    // Nombre de tickets terminés
	Financials       *FinancialSummary `json:"financials,omitempty"`
}

// WBSDependency links two WBS nodes in the Gantt chart (e.g. Finish-to-Start).
type WBSDependency struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	PredecessorID string `json:"predecessor_id"`
	SuccessorID   string `json:"successor_id"`
	DepType       string `json:"dep_type"` // "FS" (Fin-Début), "SS" (Début-Début), "FF" (Fin-Fin), "SF" (Début-Fin)
	LagDays       int    `json:"lag_days"` // Décalage (+/- jours)
}

// Ticket represents an operational item in the Kanban / Ticket tracker.
// Hybrid Free model: Always attached to a Project (impacts Project financials),
// OPTIONALLY pinned to a WBSNode (WBSNodeID != "") to link with the Gantt timeline and phase budget,
// and OPTIONALLY attached to a Parent Ticket (ParentTicketID != "") to form a parent/child sub-ticket hierarchy.
type Ticket struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	WBSNodeID      string    `json:"wbs_node_id"`       // Optional: ID of the pinned Phase/Task/Milestone in Gantt
	ParentTicketID string    `json:"parent_ticket_id"`  // Optional: ID of the parent Ticket (for sub-tickets)
	TicketKey      string    `json:"ticket_key"`        // e.g., "AIG-101"
	Title          string    `json:"title"`             // Titre du ticket
	Description    string    `json:"description"`       // Description détaillée (Markdown)
	TicketType     string    `json:"ticket_type"`       // "task", "bug", "feature", "risk", "question"
	Status         string    `json:"status"`            // "backlog", "todo", "in_progress", "review", "done"
	Priority       string    `json:"priority"`          // "low", "medium", "high", "critical"
	AssigneeID     string    `json:"assignee_id"`       // Intervenant assigné
	EstimatedHours float64   `json:"estimated_hours"`   // Estimation initiale (heures)
	LoggedHours    float64   `json:"logged_hours"`      // Temps consommé total (calculé via TimeLogs ou saisi)
	RemainingHours float64   `json:"remaining_hours"`   // Reste à faire - RAF (heures)
	FixedCost      float64   `json:"fixed_cost"`        // Coût additionnel fixe (licence, achat, prestation €)
	DueDate        string    `json:"due_date"`          // Échéance propre ou héritée du Gantt (YYYY-MM-DD)
	Tags           string    `json:"tags"`              // Tags séparés par virgule
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Enriched / Computed fields
	WBSNodeTitle          string  `json:"wbs_node_title,omitempty"`
	WBSNodeCode           string  `json:"wbs_node_code,omitempty"`
	WBSNodeEndDate        string  `json:"wbs_node_end_date,omitempty"`
	ParentTicketKey       string  `json:"parent_ticket_key,omitempty"`
	ParentTicketTitle     string  `json:"parent_ticket_title,omitempty"`
	SubTicketCnt          int     `json:"sub_ticket_cnt"`
	DoneSubTicketCnt      int     `json:"done_sub_ticket_cnt"`
	ChildrenConsumedCost  float64 `json:"children_consumed_cost"`
	ChildrenRemainingCost float64 `json:"children_remaining_cost"`
	ChildrenLandingCost   float64 `json:"children_landing_cost"`
	ConsolidatedLanding   float64 `json:"consolidated_landing"` // Coût atterrissage Parent + tous ses Sous-tickets
	AssigneeName          string  `json:"assignee_name,omitempty"`
	EffectiveRate         float64 `json:"effective_rate"`       // Taux horaire appliqué (€/h)
	InitialCost           float64 `json:"initial_cost"`         // (EstimatedHours * Rate) + FixedCost
	ConsumedCost          float64 `json:"consumed_cost"`        // Coût consommé réel (€)
	RemainingCost         float64 `json:"remaining_cost"`       // Coût du Reste à faire (RAF €)
	LandingCost           float64 `json:"landing_cost"`         // Atterrissage = ConsumedCost + RemainingCost (€)
	CostVariance          float64 `json:"cost_variance"`        // Écart = InitialCost - LandingCost (€)
	IsOverBudget          bool    `json:"is_over_budget"`       // LandingCost > InitialCost
	IsLateOnTimeline      bool    `json:"is_late_on_timeline"`  // Si DueDate > WBSNodeEndDate
}

// TimeLog represents a time entry logged on a ticket by a member.
type TimeLog struct {
	ID                 string    `json:"id"`
	TicketID           string    `json:"ticket_id"`
	MemberID           string    `json:"member_id"`
	MemberName         string    `json:"member_name,omitempty"`
	Hours              float64   `json:"hours"`
	HourlyRateSnapshot float64   `json:"hourly_rate_snapshot"` // Taux horaire au moment du pointage
	Cost               float64   `json:"cost"`                 // Hours * HourlyRateSnapshot
	WorkDate           string    `json:"work_date"`            // YYYY-MM-DD
	Comment            string    `json:"comment"`
	CreatedAt          time.Time `json:"created_at"`
}

// FinancialSummary consolidates Budget, Consumed, Remaining (RAF), and Landing (Atterrissage).
type FinancialSummary struct {
	InitialBudget    float64 `json:"initial_budget"`    // Budget initial alloué (€)
	EstimatedCost    float64 `json:"estimated_cost"`    // Somme des estimations initiales des tickets (€)
	ConsumedCost     float64 `json:"consumed_cost"`     // Consommé réel à date (€)
	RemainingCost    float64 `json:"remaining_cost"`    // Valorisation du Reste à faire - RAF (€)
	LandingCost      float64 `json:"landing_cost"`      // Atterrissage prévu = Consommé + RAF (€)
	Variance         float64 `json:"variance"`          // Écart à terminaison = Budget Initial - Atterrissage (€)
	VariancePercent  float64 `json:"variance_percent"`  // Écart en % du budget
	ConsumptionRatio float64 `json:"consumption_ratio"` // % du budget déjà consommé
	LandingRatio     float64 `json:"landing_ratio"`     // % du budget à l'atterrissage

	EstimatedHours   float64 `json:"estimated_hours"`   // Heures estimées totales
	LoggedHours      float64 `json:"logged_hours"`      // Heures pointées (consommées)
	RemainingHours   float64 `json:"remaining_hours"`   // Heures de Reste à faire (RAF)
	LandingHours     float64 `json:"landing_hours"`     // Heures à l'atterrissage (Logged + Remaining)

	UnpinnedConsumed float64 `json:"unpinned_consumed"` // Consommé des tickets rattachés au projet hors Gantt
	UnpinnedRemaining float64 `json:"unpinned_remaining"` // RAF des tickets hors Gantt
	UnpinnedLanding  float64 `json:"unpinned_landing"`  // Atterrissage des tickets hors Gantt
	HealthStatus     string  `json:"health_status"`     // "healthy" (vert), "warning" (orange), "critical" (rouge)
}

// DocPage represents a documentation / wiki page attached to a project.
type DocPage struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	ParentID        string    `json:"parent_id"` // Optional parent page for tree hierarchy
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Category        string    `json:"category"`          // "spec", "architecture", "meeting", "guide", "risk"
	ContentMD       string    `json:"content_md"`        // Contenu Markdown (supporte #TICKET-KEY)
	LinkedTicketIDs string    `json:"linked_ticket_ids"` // CSV or JSON of linked ticket IDs
	LinkedWBSIDs    string    `json:"linked_wbs_ids"`    // CSV or JSON of linked WBS node IDs
	OrderIndex      int       `json:"order_index"`
	UpdatedAt       time.Time `json:"updated_at"`
}

