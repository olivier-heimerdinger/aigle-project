// Package models définit les structures de données du serveur SaaS Aigle.
// Il intègre l'isolation Multi-Tenant (Workspaces), la gestion des utilisateurs et rôles RBAC,
// les permissions fines par projet (Maître du projet, Contributeur, Exécutant, Lecteur),
// la protection 2FA TOTP par projet, le journal d'audit de sécurité (AuditLog) et les entités métier.
package models

import "time"

// Rôles RBAC globaux disponibles sur la plateforme SaaS.
const (
	RoleSuperAdmin = "superadmin" // Administrateur global de la plateforme (accès Console Admin + tous les tenants)
	RoleAdmin      = "admin"      // Administrateur d'un Workspace/Tenant
	RoleManager    = "manager"    // Chef de projet (peut créer/modifier projets, budgets, planning et tickets)
	RoleMember     = "member"     // Collaborateur (peut créer/déplacer des tickets, pointer des heures, éditer le wiki)
	RoleViewer     = "viewer"     // Lecture seule (client externe ou observateur)
)

// Rôles prédéfinis au niveau d'un Projet (avec permissions granulaires ajustables).
const (
	ProjectRoleOwner       = "owner"       // Maître du Projet (Owner) : contrôle total + gestion des droits + 2FA
	ProjectRoleManager     = "manager"     // Chef de Projet : gestion complète du planning, tickets, budget et membres
	ProjectRoleContributor = "contributor" // Contributeur : lecture, création/édition de tickets, exécution, Gantt, Wiki
	ProjectRoleExecutor    = "executor"    // Exécutant : lecture + exécution (déplacer les tickets Kanban, pointer des heures, RAF)
	ProjectRoleViewer      = "viewer"      // Lecteur : consultation seule
)

// Workspace représente un espace client isolé (Multi-Tenant) dans le SaaS.
type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`         // ex: "Studio Heimerdinger", "Client Alpha"
	Slug        string    `json:"slug"`         // ex: "default", "client-alpha"
	Description string    `json:"description"`  // Description de l'organisation
	Status      string    `json:"status"`       // "active" ou "suspended"
	MaxProjects int       `json:"max_projects"` // Quota de projets autorisés (0 = illimité)
	MaxUsers    int       `json:"max_users"`    // Quota d'utilisateurs (0 = illimité)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Statistiques calculées pour la Console d'Administration
	ProjectCount int     `json:"project_count"`
	MemberCount  int     `json:"member_count"`
	TicketCount  int     `json:"ticket_count"`
	TotalBudget  float64 `json:"total_budget"`
}

// User représente un compte utilisateur authentifié via Authelia (SSO ForwardAuth) ou créé par l'administrateur.
type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`        // Identifiant Authelia (Remote-User)
	Email          string    `json:"email"`           // Email Authelia (Remote-Email)
	DisplayName    string    `json:"display_name"`    // Nom complet (Remote-Name)
	Role           string    `json:"role"`            // "superadmin", "admin", "manager", "member", "viewer"
	Groups         string    `json:"groups"`          // Groupes Authelia bruts (Remote-Groups)
	WorkspaceID    string    `json:"workspace_id"`    // Workspace principal assigné
	IsActive       bool      `json:"is_active"`       // Permet de bloquer un utilisateur immédiatement dans Aigle
	TOTPEnabled    bool      `json:"totp_enabled"`    // Indique si l'utilisateur a activé son 2FA TOTP
	TOTPVerifiedAt time.Time `json:"totp_verified_at"`// Horodatage de la dernière validation 2FA TOTP réussie
	LastLoginAt    time.Time `json:"last_login_at"`
	CreatedAt      time.Time `json:"created_at"`

	// Champs internes jamais sérialisés en JSON vers le client
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
}

// ProjectMember associe un utilisateur à un projet avec un rôle prédéfini et 8 permissions fines.
type ProjectMember struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	UserID           string    `json:"user_id"`
	UserEmail        string    `json:"user_email"`
	UserName         string    `json:"user_name"`
	ProjectRole      string    `json:"project_role"`       // "owner", "manager", "contributor", "executor", "viewer"
	CanRead          bool      `json:"can_read"`           // Droit de lecture du projet
	CanWriteTickets  bool      `json:"can_write_tickets"`  // Créer et modifier des tickets / sous-tickets
	CanDeleteTickets bool      `json:"can_delete_tickets"` // Supprimer des tickets
	CanExecute       bool      `json:"can_execute"`        // Exécution : déplacer les tickets Kanban, pointer des heures, ajuster le RAF
	CanEditGantt     bool      `json:"can_edit_gantt"`     // Modifier le planning MS Project (WBS & dépendances)
	CanEditWiki      bool      `json:"can_edit_wiki"`      // Créer / éditer / supprimer des pages Wiki
	CanViewFinance   bool      `json:"can_view_finance"`   // Consulter et gérer l'environnement financier (EVM, budgets, TJM)
	CanManageMembers bool      `json:"can_manage_members"` // Gérer les maîtres du projet, les membres, leurs droits et la 2FA du projet
	CreatedAt        time.Time `json:"created_at"`
}

// PresetPermissionsForRole retourne les permissions par défaut correspondant à un rôle projet prédéfini.
func PresetPermissionsForRole(role string) ProjectMember {
	switch role {
	case ProjectRoleOwner, ProjectRoleManager:
		return ProjectMember{
			ProjectRole:      role,
			CanRead:          true,
			CanWriteTickets:  true,
			CanDeleteTickets: true,
			CanExecute:       true,
			CanEditGantt:     true,
			CanEditWiki:      true,
			CanViewFinance:   true,
			CanManageMembers: true,
		}
	case ProjectRoleContributor:
		return ProjectMember{
			ProjectRole:      ProjectRoleContributor,
			CanRead:          true,
			CanWriteTickets:  true,
			CanDeleteTickets: false,
			CanExecute:       true,
			CanEditGantt:     true,
			CanEditWiki:      true,
			CanViewFinance:   true,
			CanManageMembers: false,
		}
	case ProjectRoleExecutor:
		return ProjectMember{
			ProjectRole:      ProjectRoleExecutor,
			CanRead:          true,
			CanWriteTickets:  false,
			CanDeleteTickets: false,
			CanExecute:       true,
			CanEditGantt:     false,
			CanEditWiki:      false,
			CanViewFinance:   false,
			CanManageMembers: false,
		}
	default: // ProjectRoleViewer
		return ProjectMember{
			ProjectRole:      ProjectRoleViewer,
			CanRead:          true,
			CanWriteTickets:  false,
			CanDeleteTickets: false,
			CanExecute:       false,
			CanEditGantt:     false,
			CanEditWiki:      false,
			CanViewFinance:   false,
			CanManageMembers: false,
		}
	}
}

// AuditLog enregistre de manière immuable chaque action sensible ou administrative pour la conformité et la sécurité.
type AuditLog struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserEmail   string    `json:"user_email"`
	UserRole    string    `json:"user_role"`
	Action      string    `json:"action"`      // ex: "PROJECT_CREATE", "PASSWORD_CHANGE", "PROJECT_TOTP_VERIFY", "PROJECT_MEMBER_SAVE"
	TargetType  string    `json:"target_type"` // "workspace", "user", "project", "wbs", "ticket", "doc"
	TargetID    string    `json:"target_id"`
	Details     string    `json:"details"`
	IPAddress   string    `json:"ip_address"`
	CreatedAt   time.Time `json:"created_at"`
}

// SystemOverview regroupe les métriques globales de la plateforme pour le tableau de bord d'administration.
type SystemOverview struct {
	AppVersion      string      `json:"app_version"`
	AuthMode        string      `json:"auth_mode"`
	PasswordMode    string      `json:"password_mode"`
	DBDriver        string      `json:"db_driver"`
	AppDomain       string      `json:"app_domain"`
	AuthDomain      string      `json:"auth_domain"`
	TotalWorkspaces int         `json:"total_workspaces"`
	TotalUsers      int         `json:"total_users"`
	TotalProjects   int         `json:"total_projects"`
	TotalTickets    int         `json:"total_tickets"`
	TotalBudget     float64     `json:"total_budget"`
	TotalLanding    float64     `json:"total_landing"`
	Workspaces      []Workspace `json:"workspaces"`
	Users           []User      `json:"users"`
	RecentAuditLogs []AuditLog  `json:"recent_audit_logs"`
}

// Member représente un intervenant (ressource) valorisé financièrement dans un Workspace.
type Member struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	HourlyRate  float64   `json:"hourly_rate"`
	DailyRate   float64   `json:"daily_rate"`
	AvatarColor string    `json:"avatar_color"`
	CreatedAt   time.Time `json:"created_at"`
}

// Project représente un projet rattaché à un Workspace avec son budget, sa timeline, ses maîtres/membres et sa politique 2FA TOTP.
type Project struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	ClientName        string    `json:"client_name"`
	Status            string    `json:"status"`
	StartDate         string    `json:"start_date"`
	TargetEndDate     string    `json:"target_end_date"`
	InitialBudget     float64   `json:"initial_budget"`
	DefaultHourlyRate float64   `json:"default_hourly_rate"`
	Currency          string    `json:"currency"`
	OwnerUserID       string    `json:"owner_user_id"` // ID de l'utilisateur Maître principal du projet
	OwnerName         string    `json:"owner_name,omitempty"`
	RequireTOTP       bool      `json:"require_totp"`  // Si true, exige la validation 2FA TOTP (Authelia + Aigle) pour accéder au projet
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	Financials     *FinancialSummary `json:"financials,omitempty"`
	ProjectMembers []ProjectMember   `json:"project_members,omitempty"`
	MyPermissions  *ProjectMember    `json:"my_permissions,omitempty"`
}

// WBSNode représente une Phase, Tâche ou Jalon dans le planning MS Project / Gantt.
type WBSNode struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"project_id"`
	ParentID         string  `json:"parent_id"`
	WBSCode          string  `json:"wbs_code"`
	Title            string  `json:"title"`
	NodeType         string  `json:"node_type"` // "phase", "task", "milestone"
	StartDate        string  `json:"start_date"`
	EndDate          string  `json:"end_date"`
	DurationDays     int     `json:"duration_days"`
	Progress         float64 `json:"progress"`
	AllocatedBudget  float64 `json:"allocated_budget"`
	EstimatedHours   float64 `json:"estimated_hours"`
	AssignedMemberID string  `json:"assigned_member_id"`
	OrderIndex       int     `json:"order_index"`

	// Champs calculés par les moteurs CPM & Financier
	IsCritical      bool              `json:"is_critical"`
	TotalSlackDays  int               `json:"total_slack_days"`
	EarlyStart      string            `json:"early_start"`
	EarlyFinish     string            `json:"early_finish"`
	LateStart       string            `json:"late_start"`
	LateFinish      string            `json:"late_finish"`
	PinnedTicketCnt int               `json:"pinned_ticket_cnt"`
	DoneTicketCnt   int               `json:"done_ticket_cnt"`
	Financials      *FinancialSummary `json:"financials,omitempty"`
}

// WBSDependency relie deux lignes WBS dans le diagramme de Gantt (FS, SS, FF, SF).
type WBSDependency struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	PredecessorID string `json:"predecessor_id"`
	SuccessorID   string `json:"successor_id"`
	DepType       string `json:"dep_type"`
	LagDays       int    `json:"lag_days"`
}

// Ticket représente une carte Kanban / Ticket opérationnel (Modèle Hybride Libre + Sous-tickets Enfants).
type Ticket struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	WBSNodeID      string    `json:"wbs_node_id"`
	ParentTicketID string    `json:"parent_ticket_id"`
	TicketKey      string    `json:"ticket_key"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	TicketType     string    `json:"ticket_type"`
	Status         string    `json:"status"`
	Priority       string    `json:"priority"`
	AssigneeID     string    `json:"assignee_id"`
	EstimatedHours float64   `json:"estimated_hours"`
	LoggedHours    float64   `json:"logged_hours"`
	RemainingHours float64   `json:"remaining_hours"`
	FixedCost      float64   `json:"fixed_cost"`
	DueDate        string    `json:"due_date"`
	Tags           string    `json:"tags"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Champs enrichis par le moteur Financier
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
	ConsolidatedLanding   float64 `json:"consolidated_landing"`
	AssigneeName          string  `json:"assignee_name,omitempty"`
	EffectiveRate         float64 `json:"effective_rate"`
	InitialCost           float64 `json:"initial_cost"`
	ConsumedCost          float64 `json:"consumed_cost"`
	RemainingCost         float64 `json:"remaining_cost"`
	LandingCost           float64 `json:"landing_cost"`
	CostVariance          float64 `json:"cost_variance"`
	IsOverBudget          bool    `json:"is_over_budget"`
	IsLateOnTimeline      bool    `json:"is_late_on_timeline"`
}

// TimeLog représente un pointage d'heures sur un ticket.
type TimeLog struct {
	ID                 string    `json:"id"`
	TicketID           string    `json:"ticket_id"`
	MemberID           string    `json:"member_id"`
	MemberName         string    `json:"member_name,omitempty"`
	Hours              float64   `json:"hours"`
	HourlyRateSnapshot float64   `json:"hourly_rate_snapshot"`
	Cost               float64   `json:"cost"`
	WorkDate           string    `json:"work_date"`
	Comment            string    `json:"comment"`
	CreatedAt          time.Time `json:"created_at"`
}

// FinancialSummary consolide Budget Initial, Consommé, Reste à faire (RAF) et Atterrissage (EAC).
type FinancialSummary struct {
	InitialBudget     float64 `json:"initial_budget"`
	EstimatedCost     float64 `json:"estimated_cost"`
	ConsumedCost      float64 `json:"consumed_cost"`
	RemainingCost     float64 `json:"remaining_cost"`
	LandingCost       float64 `json:"landing_cost"`
	Variance          float64 `json:"variance"`
	VariancePercent   float64 `json:"variance_percent"`
	ConsumptionRatio  float64 `json:"consumption_ratio"`
	LandingRatio      float64 `json:"landing_ratio"`
	EstimatedHours    float64 `json:"estimated_hours"`
	LoggedHours       float64 `json:"logged_hours"`
	RemainingHours    float64 `json:"remaining_hours"`
	LandingHours      float64 `json:"landing_hours"`
	UnpinnedConsumed  float64 `json:"unpinned_consumed"`
	UnpinnedRemaining float64 `json:"unpinned_remaining"`
	UnpinnedLanding   float64 `json:"unpinned_landing"`
	HealthStatus      string  `json:"health_status"`
}

// DocPage représente une page de documentation Markdown du Wiki projet.
type DocPage struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	ParentID        string    `json:"parent_id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	Category        string    `json:"category"`
	ContentMD       string    `json:"content_md"`
	LinkedTicketIDs string    `json:"linked_ticket_ids"`
	LinkedWBSIDs    string    `json:"linked_wbs_ids"`
	OrderIndex      int       `json:"order_index"`
	UpdatedAt       time.Time `json:"updated_at"`
}
