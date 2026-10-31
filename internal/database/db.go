package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aigle/aigle/internal/config"
	"github.com/aigle/aigle/internal/models"
	_ "modernc.org/sqlite"
)

// Store wraps the SQL connection and provides repository methods for Aigle.
type Store struct {
	DB     *sql.DB
	Driver string
}

// NewID generates a compact unique identifier.
func NewID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	if prefix != "" {
		return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
	}
	return hex.EncodeToString(b)
}

// Open initializes the SQLite (or Postgres) connection, runs schema migrations, and seeds initial demo data if empty.
func Open(cfg *config.Config) (*Store, error) {
	driver := cfg.DBDriver
	dsn := cfg.DBDSN

	if driver == "" || driver == "sqlite" || driver == "sqlite3" {
		driver = "sqlite"
		if dsn != ":memory:" && !strings.Contains(dsn, "_pragma") {
			sep := "?"
			if strings.Contains(dsn, "?") {
				sep = "&"
			}
			dsn = dsn + sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
		}
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("ouverture DB (%s): %w", driver, err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping DB: %w", err)
	}

	store := &Store{DB: db, Driver: driver}
	if err := store.Migrate(); err != nil {
		return nil, fmt.Errorf("migrations DB: %w", err)
	}

	if err := store.SeedIfNeeded(); err != nil {
		return nil, fmt.Errorf("seed DB: %w", err)
	}

	return store, nil
}

// Migrate creates all required tables if they do not exist yet.
func (s *Store) Migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS workspaces (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	slug TEXT NOT NULL,
	mode TEXT NOT NULL DEFAULT 'local',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS members (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	hourly_rate REAL NOT NULL DEFAULT 65.0,
	avatar_color TEXT NOT NULL DEFAULT '#3b82f6',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	code TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	client_name TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'active',
	start_date TEXT NOT NULL,
	target_end_date TEXT NOT NULL,
	initial_budget REAL NOT NULL DEFAULT 0,
	default_hourly_rate REAL NOT NULL DEFAULT 65.0,
	currency TEXT NOT NULL DEFAULT 'EUR',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS wbs_nodes (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	parent_id TEXT NOT NULL DEFAULT '',
	wbs_code TEXT NOT NULL DEFAULT '1',
	title TEXT NOT NULL,
	node_type TEXT NOT NULL DEFAULT 'task',
	start_date TEXT NOT NULL,
	end_date TEXT NOT NULL,
	duration_days INTEGER NOT NULL DEFAULT 1,
	progress REAL NOT NULL DEFAULT 0,
	allocated_budget REAL NOT NULL DEFAULT 0,
	estimated_hours REAL NOT NULL DEFAULT 0,
	assigned_member_id TEXT NOT NULL DEFAULT '',
	order_index INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS wbs_dependencies (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	predecessor_id TEXT NOT NULL,
	successor_id TEXT NOT NULL,
	dep_type TEXT NOT NULL DEFAULT 'FS',
	lag_days INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tickets (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	wbs_node_id TEXT NOT NULL DEFAULT '',
	parent_ticket_id TEXT NOT NULL DEFAULT '',
	ticket_key TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	ticket_type TEXT NOT NULL DEFAULT 'task',
	status TEXT NOT NULL DEFAULT 'todo',
	priority TEXT NOT NULL DEFAULT 'medium',
	assignee_id TEXT NOT NULL DEFAULT '',
	estimated_hours REAL NOT NULL DEFAULT 0,
	logged_hours REAL NOT NULL DEFAULT 0,
	remaining_hours REAL NOT NULL DEFAULT 0,
	fixed_cost REAL NOT NULL DEFAULT 0,
	due_date TEXT NOT NULL DEFAULT '',
	tags TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS time_logs (
	id TEXT PRIMARY KEY,
	ticket_id TEXT NOT NULL,
	member_id TEXT NOT NULL DEFAULT '',
	hours REAL NOT NULL DEFAULT 0,
	hourly_rate_snapshot REAL NOT NULL DEFAULT 0,
	work_date TEXT NOT NULL,
	comment TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS doc_pages (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	parent_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	slug TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT 'spec',
	content_md TEXT NOT NULL DEFAULT '',
	linked_ticket_ids TEXT NOT NULL DEFAULT '',
	linked_wbs_ids TEXT NOT NULL DEFAULT '',
	order_index INTEGER NOT NULL DEFAULT 0,
	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`
	_, err := s.DB.Exec(schema)
	if err != nil {
		return err
	}
	// Migration évolutive : ajoute parent_ticket_id si la base existait déjà avant cette fonctionnalité
	_, _ = s.DB.Exec(`ALTER TABLE tickets ADD COLUMN parent_ticket_id TEXT NOT NULL DEFAULT ''`)
	return nil
}

// GetMembers returns all members in the workspace.
func (s *Store) GetMembers() ([]models.Member, error) {
	rows, err := s.DB.Query(`SELECT id, workspace_id, name, email, role, hourly_rate, avatar_color, created_at FROM members ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.Name, &m.Email, &m.Role, &m.HourlyRate, &m.AvatarColor, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.DailyRate = m.HourlyRate * 8.0
		list = append(list, m)
	}
	return list, nil
}

// CreateMember inserts a new team member with their hourly rate.
func (s *Store) CreateMember(m *models.Member) error {
	if m.ID == "" {
		m.ID = NewID("mem")
	}
	if m.WorkspaceID == "" {
		m.WorkspaceID = "ws_default"
	}
	if m.AvatarColor == "" {
		m.AvatarColor = "#6366f1"
	}
	m.CreatedAt = time.Now()
	_, err := s.DB.Exec(
		`INSERT INTO members (id, workspace_id, name, email, role, hourly_rate, avatar_color, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.WorkspaceID, m.Name, m.Email, m.Role, m.HourlyRate, m.AvatarColor, m.CreatedAt,
	)
	m.DailyRate = m.HourlyRate * 8.0
	return err
}

// UpdateMember updates a member's rate, role, and details.
func (s *Store) UpdateMember(m *models.Member) error {
	_, err := s.DB.Exec(
		`UPDATE members SET name = ?, email = ?, role = ?, hourly_rate = ?, avatar_color = ? WHERE id = ?`,
		m.Name, m.Email, m.Role, m.HourlyRate, m.AvatarColor, m.ID,
	)
	m.DailyRate = m.HourlyRate * 8.0
	return err
}

// GetProjects returns all projects.
func (s *Store) GetProjects() ([]*models.Project, error) {
	rows, err := s.DB.Query(`
		SELECT id, workspace_id, code, name, description, client_name, status, start_date, target_end_date, initial_budget, default_hourly_rate, currency, created_at, updated_at
		FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Project
	for rows.Next() {
		p := &models.Project{}
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Code, &p.Name, &p.Description, &p.ClientName, &p.Status, &p.StartDate, &p.TargetEndDate, &p.InitialBudget, &p.DefaultHourlyRate, &p.Currency, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

// GetProject returns a single project by ID.
func (s *Store) GetProject(id string) (*models.Project, error) {
	p := &models.Project{}
	err := s.DB.QueryRow(`
		SELECT id, workspace_id, code, name, description, client_name, status, start_date, target_end_date, initial_budget, default_hourly_rate, currency, created_at, updated_at
		FROM projects WHERE id = ?`, id).Scan(
		&p.ID, &p.WorkspaceID, &p.Code, &p.Name, &p.Description, &p.ClientName, &p.Status, &p.StartDate, &p.TargetEndDate, &p.InitialBudget, &p.DefaultHourlyRate, &p.Currency, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// CreateProject creates a new project.
func (s *Store) CreateProject(p *models.Project) error {
	if p.ID == "" {
		p.ID = NewID("prj")
	}
	if p.WorkspaceID == "" {
		p.WorkspaceID = "ws_default"
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Currency == "" {
		p.Currency = "EUR"
	}
	if p.DefaultHourlyRate <= 0 {
		p.DefaultHourlyRate = 65.0
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := s.DB.Exec(`
		INSERT INTO projects (id, workspace_id, code, name, description, client_name, status, start_date, target_end_date, initial_budget, default_hourly_rate, currency, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.WorkspaceID, strings.ToUpper(p.Code), p.Name, p.Description, p.ClientName, p.Status, p.StartDate, p.TargetEndDate, p.InitialBudget, p.DefaultHourlyRate, p.Currency, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// UpdateProject updates a project's metadata and budget.
func (s *Store) UpdateProject(p *models.Project) error {
	p.UpdatedAt = time.Now()
	_, err := s.DB.Exec(`
		UPDATE projects SET code = ?, name = ?, description = ?, client_name = ?, status = ?, start_date = ?, target_end_date = ?, initial_budget = ?, default_hourly_rate = ?, currency = ?, updated_at = ?
		WHERE id = ?`,
		strings.ToUpper(p.Code), p.Name, p.Description, p.ClientName, p.Status, p.StartDate, p.TargetEndDate, p.InitialBudget, p.DefaultHourlyRate, p.Currency, p.UpdatedAt, p.ID,
	)
	return err
}

// DeleteProject removes a project and all its associated entities.
func (s *Store) DeleteProject(id string) error {
	_, _ = s.DB.Exec(`DELETE FROM time_logs WHERE ticket_id IN (SELECT id FROM tickets WHERE project_id = ?)`, id)
	_, _ = s.DB.Exec(`DELETE FROM tickets WHERE project_id = ?`, id)
	_, _ = s.DB.Exec(`DELETE FROM wbs_dependencies WHERE project_id = ?`, id)
	_, _ = s.DB.Exec(`DELETE FROM wbs_nodes WHERE project_id = ?`, id)
	_, _ = s.DB.Exec(`DELETE FROM doc_pages WHERE project_id = ?`, id)
	_, err := s.DB.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// GetWBSNodes returns all WBS nodes for a project ordered by order_index.
func (s *Store) GetWBSNodes(projectID string) ([]*models.WBSNode, error) {
	rows, err := s.DB.Query(`
		SELECT id, project_id, parent_id, wbs_code, title, node_type, start_date, end_date, duration_days, progress, allocated_budget, estimated_hours, assigned_member_id, order_index
		FROM wbs_nodes WHERE project_id = ? ORDER BY order_index ASC, wbs_code ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*models.WBSNode
	for rows.Next() {
		n := &models.WBSNode{}
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.ParentID, &n.WBSCode, &n.Title, &n.NodeType, &n.StartDate, &n.EndDate, &n.DurationDays, &n.Progress, &n.AllocatedBudget, &n.EstimatedHours, &n.AssignedMemberID, &n.OrderIndex); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// SaveWBSNode inserts or updates a WBSNode (Upsert).
func (s *Store) SaveWBSNode(n *models.WBSNode) error {
	if n.ID == "" {
		n.ID = NewID("wbs")
	}
	_, err := s.DB.Exec(`
		INSERT INTO wbs_nodes (id, project_id, parent_id, wbs_code, title, node_type, start_date, end_date, duration_days, progress, allocated_budget, estimated_hours, assigned_member_id, order_index)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			parent_id = excluded.parent_id,
			wbs_code = excluded.wbs_code,
			title = excluded.title,
			node_type = excluded.node_type,
			start_date = excluded.start_date,
			end_date = excluded.end_date,
			duration_days = excluded.duration_days,
			progress = excluded.progress,
			allocated_budget = excluded.allocated_budget,
			estimated_hours = excluded.estimated_hours,
			assigned_member_id = excluded.assigned_member_id,
			order_index = excluded.order_index`,
		n.ID, n.ProjectID, n.ParentID, n.WBSCode, n.Title, n.NodeType, n.StartDate, n.EndDate, n.DurationDays, n.Progress, n.AllocatedBudget, n.EstimatedHours, n.AssignedMemberID, n.OrderIndex,
	)
	return err
}

// DeleteWBSNode deletes a WBS node, unpins any attached tickets, and deletes its dependencies.
func (s *Store) DeleteWBSNode(id string) error {
	_, _ = s.DB.Exec(`UPDATE tickets SET wbs_node_id = '' WHERE wbs_node_id = ?`, id)
	_, _ = s.DB.Exec(`UPDATE wbs_nodes SET parent_id = '' WHERE parent_id = ?`, id)
	_, _ = s.DB.Exec(`DELETE FROM wbs_dependencies WHERE predecessor_id = ? OR successor_id = ?`, id, id)
	_, err := s.DB.Exec(`DELETE FROM wbs_nodes WHERE id = ?`, id)
	return err
}

// GetWBSDependencies returns all dependencies for a project.
func (s *Store) GetWBSDependencies(projectID string) ([]models.WBSDependency, error) {
	rows, err := s.DB.Query(`
		SELECT id, project_id, predecessor_id, successor_id, dep_type, lag_days
		FROM wbs_dependencies WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.WBSDependency
	for rows.Next() {
		var d models.WBSDependency
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.PredecessorID, &d.SuccessorID, &d.DepType, &d.LagDays); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, nil
}

// CreateWBSDependency creates a link between two WBS nodes.
func (s *Store) CreateWBSDependency(d *models.WBSDependency) error {
	if d.ID == "" {
		d.ID = NewID("dep")
	}
	if d.DepType == "" {
		d.DepType = "FS"
	}
	_, err := s.DB.Exec(`
		INSERT INTO wbs_dependencies (id, project_id, predecessor_id, successor_id, dep_type, lag_days)
		VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.PredecessorID, d.SuccessorID, d.DepType, d.LagDays,
	)
	return err
}

// DeleteWBSDependency removes a dependency link.
func (s *Store) DeleteWBSDependency(id string) error {
	_, err := s.DB.Exec(`DELETE FROM wbs_dependencies WHERE id = ?`, id)
	return err
}

// GetTickets returns all tickets for a project.
func (s *Store) GetTickets(projectID string) ([]*models.Ticket, error) {
	rows, err := s.DB.Query(`
		SELECT id, project_id, wbs_node_id, parent_ticket_id, ticket_key, title, description, ticket_type, status, priority, assignee_id, estimated_hours, logged_hours, remaining_hours, fixed_cost, due_date, tags, created_at, updated_at
		FROM tickets WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Ticket
	for rows.Next() {
		t := &models.Ticket{}
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.WBSNodeID, &t.ParentTicketID, &t.TicketKey, &t.Title, &t.Description, &t.TicketType, &t.Status, &t.Priority, &t.AssigneeID, &t.EstimatedHours, &t.LoggedHours, &t.RemainingHours, &t.FixedCost, &t.DueDate, &t.Tags, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

// GetTicket returns a single ticket by ID.
func (s *Store) GetTicket(id string) (*models.Ticket, error) {
	t := &models.Ticket{}
	err := s.DB.QueryRow(`
		SELECT id, project_id, wbs_node_id, parent_ticket_id, ticket_key, title, description, ticket_type, status, priority, assignee_id, estimated_hours, logged_hours, remaining_hours, fixed_cost, due_date, tags, created_at, updated_at
		FROM tickets WHERE id = ?`, id).Scan(
		&t.ID, &t.ProjectID, &t.WBSNodeID, &t.ParentTicketID, &t.TicketKey, &t.Title, &t.Description, &t.TicketType, &t.Status, &t.Priority, &t.AssigneeID, &t.EstimatedHours, &t.LoggedHours, &t.RemainingHours, &t.FixedCost, &t.DueDate, &t.Tags, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SaveTicket inserts or updates a ticket (Upsert).
func (s *Store) SaveTicket(t *models.Ticket) error {
	now := time.Now()
	t.UpdatedAt = now
	if t.ID == "" {
		t.ID = NewID("tck")
		t.CreatedAt = now
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	// Empêcher un ticket d'être son propre parent
	if t.ParentTicketID == t.ID {
		t.ParentTicketID = ""
	}
	if t.TicketKey == "" {
		var cnt int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM tickets WHERE project_id = ?`, t.ProjectID).Scan(&cnt)
		var pCode string
		_ = s.DB.QueryRow(`SELECT code FROM projects WHERE id = ?`, t.ProjectID).Scan(&pCode)
		if pCode == "" {
			pCode = "PRJ"
		}
		t.TicketKey = fmt.Sprintf("%s-%d", pCode, cnt+1)
	}

	_, err := s.DB.Exec(`
		INSERT INTO tickets (id, project_id, wbs_node_id, parent_ticket_id, ticket_key, title, description, ticket_type, status, priority, assignee_id, estimated_hours, logged_hours, remaining_hours, fixed_cost, due_date, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			wbs_node_id = excluded.wbs_node_id,
			parent_ticket_id = excluded.parent_ticket_id,
			title = excluded.title,
			description = excluded.description,
			ticket_type = excluded.ticket_type,
			status = excluded.status,
			priority = excluded.priority,
			assignee_id = excluded.assignee_id,
			estimated_hours = excluded.estimated_hours,
			logged_hours = excluded.logged_hours,
			remaining_hours = excluded.remaining_hours,
			fixed_cost = excluded.fixed_cost,
			due_date = excluded.due_date,
			tags = excluded.tags,
			updated_at = excluded.updated_at`,
		t.ID, t.ProjectID, t.WBSNodeID, t.ParentTicketID, t.TicketKey, t.Title, t.Description, t.TicketType, t.Status, t.Priority, t.AssigneeID, t.EstimatedHours, t.LoggedHours, t.RemainingHours, t.FixedCost, t.DueDate, t.Tags, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

// DeleteTicket deletes a ticket, detaches any child sub-tickets, and removes its time logs.
func (s *Store) DeleteTicket(id string) error {
	_, _ = s.DB.Exec(`UPDATE tickets SET parent_ticket_id = '' WHERE parent_ticket_id = ?`, id)
	_, _ = s.DB.Exec(`DELETE FROM time_logs WHERE ticket_id = ?`, id)
	_, err := s.DB.Exec(`DELETE FROM tickets WHERE id = ?`, id)
	return err
}

// GetTimeLogsByProject returns a map of ticket_id -> []TimeLog for all tickets in a project.
func (s *Store) GetTimeLogsByProject(projectID string) (map[string][]models.TimeLog, error) {
	rows, err := s.DB.Query(`
		SELECT l.id, l.ticket_id, l.member_id, COALESCE(m.name, 'Intervenant'), l.hours, l.hourly_rate_snapshot, l.work_date, l.comment, l.created_at
		FROM time_logs l
		JOIN tickets t ON t.id = l.ticket_id
		LEFT JOIN members m ON m.id = l.member_id
		WHERE t.project_id = ?
		ORDER BY l.work_date DESC, l.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]models.TimeLog)
	for rows.Next() {
		var l models.TimeLog
		if err := rows.Scan(&l.ID, &l.TicketID, &l.MemberID, &l.MemberName, &l.Hours, &l.HourlyRateSnapshot, &l.WorkDate, &l.Comment, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.Cost = l.Hours * l.HourlyRateSnapshot
		result[l.TicketID] = append(result[l.TicketID], l)
	}
	return result, nil
}

// AddTimeLog logs work on a ticket and optionally updates the ticket's remaining hours (RAF).
func (s *Store) AddTimeLog(l *models.TimeLog, newRemainingHours *float64) error {
	if l.ID == "" {
		l.ID = NewID("log")
	}
	l.CreatedAt = time.Now()
	if l.WorkDate == "" {
		l.WorkDate = l.CreatedAt.Format("2006-01-02")
	}

	// Lookup member rate if snapshot not provided
	if l.HourlyRateSnapshot <= 0 && l.MemberID != "" {
		_ = s.DB.QueryRow(`SELECT hourly_rate FROM members WHERE id = ?`, l.MemberID).Scan(&l.HourlyRateSnapshot)
	}
	if l.HourlyRateSnapshot <= 0 {
		l.HourlyRateSnapshot = 65.0
	}

	_, err := s.DB.Exec(`
		INSERT INTO time_logs (id, ticket_id, member_id, hours, hourly_rate_snapshot, work_date, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.TicketID, l.MemberID, l.Hours, l.HourlyRateSnapshot, l.WorkDate, l.Comment, l.CreatedAt,
	)
	if err != nil {
		return err
	}

	// Recalculate total logged_hours on the ticket
	var totalLogged float64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(hours), 0) FROM time_logs WHERE ticket_id = ?`, l.TicketID).Scan(&totalLogged)

	if newRemainingHours != nil {
		_, err = s.DB.Exec(`UPDATE tickets SET logged_hours = ?, remaining_hours = ?, updated_at = ? WHERE id = ?`,
			totalLogged, *newRemainingHours, time.Now(), l.TicketID)
	} else {
		// Automatically deduct logged hours from remaining_hours (down to 0)
		var currRem float64
		_ = s.DB.QueryRow(`SELECT remaining_hours FROM tickets WHERE id = ?`, l.TicketID).Scan(&currRem)
		nextRem := currRem - l.Hours
		if nextRem < 0 {
			nextRem = 0
		}
		_, err = s.DB.Exec(`UPDATE tickets SET logged_hours = ?, remaining_hours = ?, updated_at = ? WHERE id = ?`,
			totalLogged, nextRem, time.Now(), l.TicketID)
	}
	return err
}

// GetDocPages returns all documentation pages for a project.
func (s *Store) GetDocPages(projectID string) ([]*models.DocPage, error) {
	rows, err := s.DB.Query(`
		SELECT id, project_id, parent_id, title, slug, category, content_md, linked_ticket_ids, linked_wbs_ids, order_index, updated_at
		FROM doc_pages WHERE project_id = ? ORDER BY order_index ASC, title ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []*models.DocPage
	for rows.Next() {
		d := &models.DocPage{}
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.ParentID, &d.Title, &d.Slug, &d.Category, &d.ContentMD, &d.LinkedTicketIDs, &d.LinkedWBSIDs, &d.OrderIndex, &d.UpdatedAt); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// SaveDocPage inserts or updates a documentation page (Upsert).
func (s *Store) SaveDocPage(d *models.DocPage) error {
	d.UpdatedAt = time.Now()
	if d.ID == "" {
		d.ID = NewID("doc")
	}
	_, err := s.DB.Exec(`
		INSERT INTO doc_pages (id, project_id, parent_id, title, slug, category, content_md, linked_ticket_ids, linked_wbs_ids, order_index, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			parent_id = excluded.parent_id,
			title = excluded.title,
			slug = excluded.slug,
			category = excluded.category,
			content_md = excluded.content_md,
			linked_ticket_ids = excluded.linked_ticket_ids,
			linked_wbs_ids = excluded.linked_wbs_ids,
			order_index = excluded.order_index,
			updated_at = excluded.updated_at`,
		d.ID, d.ProjectID, d.ParentID, d.Title, d.Slug, d.Category, d.ContentMD, d.LinkedTicketIDs, d.LinkedWBSIDs, d.OrderIndex, d.UpdatedAt,
	)
	return err
}

// DeleteDocPage deletes a documentation page.
func (s *Store) DeleteDocPage(id string) error {
	_, err := s.DB.Exec(`DELETE FROM doc_pages WHERE id = ?`, id)
	return err
}
