// Package database implémente la couche de persistance SQL Multi-Tenant du serveur SaaS Aigle.
// Sécurité :
//   - 100% des requêtes SQL utilisent des paramètres liés (Prepared/Parameterized Statements),
//     éliminant toute vulnérabilité d'injection SQL (OWASP A03).
//   - Traduction automatique des placeholders '?' vers '$1, $2, ...' lorsque PostgreSQL est utilisé.
//   - Isolation Multi-Tenant par workspace_id, permissions granulaires par projet (project_members),
//     support 2FA TOTP par utilisateur et par projet, et journalisation d'audit immuable (audit_logs).
package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aigle/aigle-server/internal/config"
	"github.com/aigle/aigle-server/internal/models"
	_ "modernc.org/sqlite"
)

// Store encapsule le pool de connexions SQL et le driver actif ("postgres" ou "sqlite").
type Store struct {
	DB     *sql.DB
	Driver string
}

// NewID génère un identifiant aléatoire cryptographiquement sûr (crypto/rand).
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	if prefix != "" {
		return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
	}
	return hex.EncodeToString(b)
}

// rebind convertit les marqueurs '?' en '$1, $2, ...' pour PostgreSQL.
func (s *Store) rebind(query string) string {
	if s.Driver != "postgres" {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 16)
	idx := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out.WriteByte('$')
			out.WriteString(strconv.Itoa(idx))
			idx++
		} else {
			out.WriteByte(query[i])
		}
	}
	return out.String()
}

func (s *Store) exec(query string, args ...any) (sql.Result, error) {
	return s.DB.Exec(s.rebind(query), args...)
}

func (s *Store) query(query string, args ...any) (*sql.Rows, error) {
	return s.DB.Query(s.rebind(query), args...)
}

func (s *Store) queryRow(query string, args ...any) *sql.Row {
	return s.DB.QueryRow(s.rebind(query), args...)
}

// Open initialise le pool de connexions SQL (PostgreSQL ou SQLite), exécute les migrations et le seed initial.
func Open(cfg *config.ServerConfig) (*Store, error) {
	driver := strings.ToLower(cfg.DBDriver)
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
	} else if driver == "postgres" || driver == "postgresql" {
		driver = "postgres"
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("ouverture DB (%s): %w", driver, err)
	}

	// Configuration du pool de connexions pour un environnement SaaS multi-utilisateurs
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Attente résiliente au démarrage (utile dans Docker Compose quand PostgreSQL finit son init)
	var pingErr error
	for attempt := 1; attempt <= 10; attempt++ {
		if pingErr = db.Ping(); pingErr == nil {
			break
		}
		time.Sleep(time.Duration(attempt*300) * time.Millisecond)
	}
	if pingErr != nil {
		return nil, fmt.Errorf("impossible de joindre la base de données (%s): %w", driver, pingErr)
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

// Migrate crée l'ensemble des tables SaaS et applique les évolutions de schéma de façon idempotente.
func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			max_projects INTEGER NOT NULL DEFAULT 50,
			max_users INTEGER NOT NULL DEFAULT 100,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'member',
			groups TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL DEFAULT 'ws_default',
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			password_hash TEXT NOT NULL DEFAULT '',
			totp_secret TEXT NOT NULL DEFAULT '',
			totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
			totp_verified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_login_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT 'ws_default',
			user_email TEXT NOT NULL DEFAULT '',
			user_role TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS members (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			name TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			hourly_rate REAL NOT NULL DEFAULT 65.0,
			avatar_color TEXT NOT NULL DEFAULT '#6366f1',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
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
			owner_user_id TEXT NOT NULL DEFAULT '',
			require_totp BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS project_members (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			user_email TEXT NOT NULL DEFAULT '',
			user_name TEXT NOT NULL DEFAULT '',
			project_role TEXT NOT NULL DEFAULT 'contributor',
			can_read BOOLEAN NOT NULL DEFAULT TRUE,
			can_write_tickets BOOLEAN NOT NULL DEFAULT TRUE,
			can_delete_tickets BOOLEAN NOT NULL DEFAULT FALSE,
			can_execute BOOLEAN NOT NULL DEFAULT TRUE,
			can_edit_gantt BOOLEAN NOT NULL DEFAULT FALSE,
			can_edit_wiki BOOLEAN NOT NULL DEFAULT TRUE,
			can_view_finance BOOLEAN NOT NULL DEFAULT FALSE,
			can_manage_members BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(project_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS wbs_nodes (
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
		)`,
		`CREATE TABLE IF NOT EXISTS wbs_dependencies (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			predecessor_id TEXT NOT NULL,
			successor_id TEXT NOT NULL,
			dep_type TEXT NOT NULL DEFAULT 'FS',
			lag_days INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS tickets (
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
		)`,
		`CREATE TABLE IF NOT EXISTS time_logs (
			id TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL,
			member_id TEXT NOT NULL DEFAULT '',
			hours REAL NOT NULL DEFAULT 0,
			hourly_rate_snapshot REAL NOT NULL DEFAULT 0,
			work_date TEXT NOT NULL,
			comment TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS doc_pages (
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
		)`,
	}

	for _, q := range stmts {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}

	// Migrations incrémentales idempotentes pour les bases PostgreSQL / SQLite déjà existantes
	alterStmts := []string{
		`ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN totp_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN totp_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE users ADD COLUMN totp_verified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE projects ADD COLUMN owner_user_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE projects ADD COLUMN require_totp BOOLEAN NOT NULL DEFAULT FALSE`,
	}
	for _, alt := range alterStmts {
		_, _ = s.DB.Exec(alt)
	}

	return nil
}

// --- Gestion des Utilisateurs SSO Authelia, Compte Personnel & 2FA TOTP ---

// SyncAuthenticatedUser synchronise en base de données l'utilisateur authentifié par Authelia ForwardAuth.
func (s *Store) SyncAuthenticatedUser(u *models.User) (*models.User, error) {
	var existing models.User
	var totpVerified sql.NullTime
	err := s.queryRow(`
		SELECT id, username, email, display_name, role, groups, workspace_id, is_active,
		       password_hash, totp_secret, totp_enabled, totp_verified_at, last_login_at, created_at
		FROM users WHERE email = ?`, u.Email).Scan(
		&existing.ID, &existing.Username, &existing.Email, &existing.DisplayName,
		&existing.Role, &existing.Groups, &existing.WorkspaceID, &existing.IsActive,
		&existing.PasswordHash, &existing.TOTPSecret, &existing.TOTPEnabled,
		&totpVerified, &existing.LastLoginAt, &existing.CreatedAt,
	)
	if totpVerified.Valid {
		existing.TOTPVerifiedAt = totpVerified.Time
	}

	now := time.Now()
	if err == sql.ErrNoRows {
		u.ID = NewID("usr")
		if u.WorkspaceID == "" {
			u.WorkspaceID = "ws_default"
		}
		u.IsActive = true
		u.LastLoginAt = now
		u.CreatedAt = now
		u.TOTPVerifiedAt = time.Time{}
		_, insertErr := s.exec(`
			INSERT INTO users (id, username, email, display_name, role, groups, workspace_id, is_active, password_hash, totp_secret, totp_enabled, totp_verified_at, last_login_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.ID, u.Username, u.Email, u.DisplayName, u.Role, u.Groups, u.WorkspaceID, u.IsActive, "", "", false, time.Unix(0, 0), u.LastLoginAt, u.CreatedAt,
		)
		return u, insertErr
	} else if err != nil {
		return u, err
	}

	existing.LastLoginAt = now
	if u.Groups != "" {
		existing.Groups = u.Groups
	}
	if u.Role == models.RoleSuperAdmin {
		existing.Role = models.RoleSuperAdmin
	}
	if existing.DisplayName == "" && u.DisplayName != "" {
		existing.DisplayName = u.DisplayName
	}
	_, _ = s.exec(`UPDATE users SET last_login_at = ?, groups = ? WHERE id = ?`,
		existing.LastLoginAt, existing.Groups, existing.ID)

	return &existing, nil
}

// GetUserByID retourne un utilisateur par son ID avec ses métadonnées 2FA et hash mot de passe.
func (s *Store) GetUserByID(id string) (*models.User, error) {
	var u models.User
	var totpVerified sql.NullTime
	err := s.queryRow(`
		SELECT id, username, email, display_name, role, groups, workspace_id, is_active,
		       password_hash, totp_secret, totp_enabled, totp_verified_at, last_login_at, created_at
		FROM users WHERE id = ?`, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Groups, &u.WorkspaceID, &u.IsActive,
		&u.PasswordHash, &u.TOTPSecret, &u.TOTPEnabled, &totpVerified, &u.LastLoginAt, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if totpVerified.Valid {
		u.TOTPVerifiedAt = totpVerified.Time
	}
	return &u, nil
}

// GetUsers retourne tous les utilisateurs enregistrés sur la plateforme SaaS.
func (s *Store) GetUsers() ([]models.User, error) {
	rows, err := s.query(`
		SELECT id, username, email, display_name, role, groups, workspace_id, is_active,
		       totp_enabled, totp_verified_at, last_login_at, created_at
		FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.User
	for rows.Next() {
		var u models.User
		var totpVerified sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Groups, &u.WorkspaceID, &u.IsActive, &u.TOTPEnabled, &totpVerified, &u.LastLoginAt, &u.CreatedAt); err != nil {
			return nil, err
		}
		if totpVerified.Valid {
			u.TOTPVerifiedAt = totpVerified.Time
		}
		list = append(list, u)
	}
	return list, nil
}

// SaveUser crée ou met à jour un utilisateur depuis la Console d'Administration.
func (s *Store) SaveUser(u *models.User) error {
	if u.ID == "" {
		u.ID = NewID("usr")
		u.CreatedAt = time.Now()
		u.LastLoginAt = u.CreatedAt
		_, err := s.exec(`
			INSERT INTO users (id, username, email, display_name, role, groups, workspace_id, is_active, password_hash, totp_secret, totp_enabled, totp_verified_at, last_login_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			u.ID, u.Username, strings.ToLower(u.Email), u.DisplayName, u.Role, u.Groups, u.WorkspaceID, u.IsActive, u.PasswordHash, "", false, time.Unix(0, 0), u.LastLoginAt, u.CreatedAt,
		)
		return err
	}
	_, err := s.exec(`
		UPDATE users SET display_name = ?, role = ?, workspace_id = ?, is_active = ?
		WHERE id = ?`,
		u.DisplayName, u.Role, u.WorkspaceID, u.IsActive, u.ID,
	)
	return err
}

// UpdateUserProfile permet à l'utilisateur connecté de modifier son nom d'affichage.
func (s *Store) UpdateUserProfile(userID, displayName string) error {
	_, err := s.exec(`UPDATE users SET display_name = ? WHERE id = ?`, displayName, userID)
	return err
}

// UpdateUserPasswordHash enregistre l'empreinte Argon2id du mot de passe en base de données.
func (s *Store) UpdateUserPasswordHash(userID, passwordHash string) error {
	_, err := s.exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

// SaveUserTOTPSecret enregistre le secret TOTP Base32 de l'utilisateur et son statut d'activation.
func (s *Store) SaveUserTOTPSecret(userID, secret string, enabled bool) error {
	verifiedAt := time.Unix(0, 0)
	if enabled {
		verifiedAt = time.Now()
	}
	_, err := s.exec(`UPDATE users SET totp_secret = ?, totp_enabled = ?, totp_verified_at = ? WHERE id = ?`,
		secret, enabled, verifiedAt, userID)
	return err
}

// MarkUserTOTPVerified horodate la réussite d'une vérification 2FA TOTP pour l'utilisateur.
func (s *Store) MarkUserTOTPVerified(userID string, verifiedAt time.Time) error {
	_, err := s.exec(`UPDATE users SET totp_verified_at = ? WHERE id = ?`, verifiedAt, userID)
	return err
}

// --- Gestion des Workspaces (Multi-Tenants SaaS) ---

func (s *Store) GetWorkspaces() ([]models.Workspace, error) {
	rows, err := s.query(`
		SELECT id, name, slug, description, status, max_projects, max_users, created_at, updated_at
		FROM workspaces ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Workspace
	for rows.Next() {
		var w models.Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.Status, &w.MaxProjects, &w.MaxUsers, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		_ = s.queryRow(`SELECT COUNT(*), COALESCE(SUM(initial_budget), 0) FROM projects WHERE workspace_id = ?`, w.ID).Scan(&w.ProjectCount, &w.TotalBudget)
		_ = s.queryRow(`SELECT COUNT(*) FROM members WHERE workspace_id = ?`, w.ID).Scan(&w.MemberCount)
		_ = s.queryRow(`SELECT COUNT(*) FROM tickets t JOIN projects p ON p.id = t.project_id WHERE p.workspace_id = ?`, w.ID).Scan(&w.TicketCount)
		list = append(list, w)
	}
	return list, nil
}

func (s *Store) SaveWorkspace(w *models.Workspace) error {
	now := time.Now()
	w.UpdatedAt = now
	if w.ID == "" {
		w.ID = NewID("ws")
		w.CreatedAt = now
	}
	if w.Status == "" {
		w.Status = "active"
	}
	if w.MaxProjects <= 0 {
		w.MaxProjects = 50
	}
	if w.MaxUsers <= 0 {
		w.MaxUsers = 100
	}
	_, err := s.exec(`
		INSERT INTO workspaces (id, name, slug, description, status, max_projects, max_users, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			slug = excluded.slug,
			description = excluded.description,
			status = excluded.status,
			max_projects = excluded.max_projects,
			max_users = excluded.max_users,
			updated_at = excluded.updated_at`,
		w.ID, w.Name, w.Slug, w.Description, w.Status, w.MaxProjects, w.MaxUsers, w.CreatedAt, w.UpdatedAt,
	)
	return err
}

// --- Journal d'Audit de Sécurité (Audit Trail) ---

func (s *Store) AddAuditLog(workspaceID, userEmail, userRole, action, targetType, targetID, details, ip string) {
	if workspaceID == "" {
		workspaceID = "ws_default"
	}
	_, _ = s.exec(`
		INSERT INTO audit_logs (id, workspace_id, user_email, user_role, action, target_type, target_id, details, ip_address, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		NewID("aud"), workspaceID, userEmail, userRole, action, targetType, targetID, details, ip, time.Now(),
	)
}

func (s *Store) GetRecentAuditLogs(limit int) ([]models.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.query(`
		SELECT id, workspace_id, user_email, user_role, action, target_type, target_id, details, ip_address, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.WorkspaceID, &l.UserEmail, &l.UserRole, &l.Action, &l.TargetType, &l.TargetID, &l.Details, &l.IPAddress, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// --- Intervenants (Members) par Workspace ---

func (s *Store) GetMembers(workspaceID string) ([]models.Member, error) {
	var rows *sql.Rows
	var err error
	if workspaceID != "" {
		rows, err = s.query(`SELECT id, workspace_id, name, email, role, hourly_rate, avatar_color, created_at FROM members WHERE workspace_id = ? ORDER BY name ASC`, workspaceID)
	} else {
		rows, err = s.query(`SELECT id, workspace_id, name, email, role, hourly_rate, avatar_color, created_at FROM members ORDER BY name ASC`)
	}
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
	_, err := s.exec(
		`INSERT INTO members (id, workspace_id, name, email, role, hourly_rate, avatar_color, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.WorkspaceID, m.Name, m.Email, m.Role, m.HourlyRate, m.AvatarColor, m.CreatedAt,
	)
	m.DailyRate = m.HourlyRate * 8.0
	return err
}

func (s *Store) UpdateMember(m *models.Member) error {
	_, err := s.exec(
		`UPDATE members SET name = ?, email = ?, role = ?, hourly_rate = ?, avatar_color = ? WHERE id = ?`,
		m.Name, m.Email, m.Role, m.HourlyRate, m.AvatarColor, m.ID,
	)
	m.DailyRate = m.HourlyRate * 8.0
	return err
}

// --- Projets, Maîtres de Projet & Droits Granulaires (TODO #2 & #3) ---

func (s *Store) GetProjects(workspaceID string) ([]*models.Project, error) {
	var rows *sql.Rows
	var err error
	if workspaceID != "" {
		rows, err = s.query(`
			SELECT p.id, p.workspace_id, p.code, p.name, p.description, p.client_name, p.status,
			       p.start_date, p.target_end_date, p.initial_budget, p.default_hourly_rate, p.currency,
			       p.owner_user_id, COALESCE(u.display_name, ''), p.require_totp, p.created_at, p.updated_at
			FROM projects p
			LEFT JOIN users u ON u.id = p.owner_user_id
			WHERE p.workspace_id = ?
			ORDER BY p.created_at DESC`, workspaceID)
	} else {
		rows, err = s.query(`
			SELECT p.id, p.workspace_id, p.code, p.name, p.description, p.client_name, p.status,
			       p.start_date, p.target_end_date, p.initial_budget, p.default_hourly_rate, p.currency,
			       p.owner_user_id, COALESCE(u.display_name, ''), p.require_totp, p.created_at, p.updated_at
			FROM projects p
			LEFT JOIN users u ON u.id = p.owner_user_id
			ORDER BY p.created_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Project
	for rows.Next() {
		p := &models.Project{}
		if err := rows.Scan(
			&p.ID, &p.WorkspaceID, &p.Code, &p.Name, &p.Description, &p.ClientName, &p.Status,
			&p.StartDate, &p.TargetEndDate, &p.InitialBudget, &p.DefaultHourlyRate, &p.Currency,
			&p.OwnerUserID, &p.OwnerName, &p.RequireTOTP, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.ProjectMembers, _ = s.GetProjectMembers(p.ID)
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) GetProject(id string) (*models.Project, error) {
	p := &models.Project{}
	err := s.queryRow(`
		SELECT p.id, p.workspace_id, p.code, p.name, p.description, p.client_name, p.status,
		       p.start_date, p.target_end_date, p.initial_budget, p.default_hourly_rate, p.currency,
		       p.owner_user_id, COALESCE(u.display_name, ''), p.require_totp, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN users u ON u.id = p.owner_user_id
		WHERE p.id = ?`, id).Scan(
		&p.ID, &p.WorkspaceID, &p.Code, &p.Name, &p.Description, &p.ClientName, &p.Status,
		&p.StartDate, &p.TargetEndDate, &p.InitialBudget, &p.DefaultHourlyRate, &p.Currency,
		&p.OwnerUserID, &p.OwnerName, &p.RequireTOTP, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.ProjectMembers, _ = s.GetProjectMembers(p.ID)
	return p, nil
}

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

	_, err := s.exec(`
		INSERT INTO projects (id, workspace_id, code, name, description, client_name, status, start_date, target_end_date, initial_budget, default_hourly_rate, currency, owner_user_id, require_totp, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.WorkspaceID, strings.ToUpper(p.Code), p.Name, p.Description, p.ClientName, p.Status, p.StartDate, p.TargetEndDate, p.InitialBudget, p.DefaultHourlyRate, p.Currency, p.OwnerUserID, p.RequireTOTP, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Si un Maître du Projet (OwnerUserID) est défini, l'inscrire automatiquement dans project_members avec le rôle 'owner'
	if p.OwnerUserID != "" {
		u, uErr := s.GetUserByID(p.OwnerUserID)
		ownerPerm := models.PresetPermissionsForRole(models.ProjectRoleOwner)
		ownerPerm.ProjectID = p.ID
		ownerPerm.UserID = p.OwnerUserID
		if uErr == nil && u != nil {
			ownerPerm.UserEmail = u.Email
			ownerPerm.UserName = u.DisplayName
		}
		_ = s.SaveProjectMember(&ownerPerm)
	}

	// Enregistre également les autres membres initiaux éventuellement fournis lors de la création
	for i := range p.ProjectMembers {
		pm := p.ProjectMembers[i]
		pm.ProjectID = p.ID
		if pm.UserID != "" {
			_ = s.SaveProjectMember(&pm)
		}
	}

	return nil
}

func (s *Store) UpdateProject(p *models.Project) error {
	p.UpdatedAt = time.Now()
	_, err := s.exec(`
		UPDATE projects SET code = ?, name = ?, description = ?, client_name = ?, status = ?, start_date = ?, target_end_date = ?, initial_budget = ?, default_hourly_rate = ?, currency = ?, owner_user_id = ?, require_totp = ?, updated_at = ?
		WHERE id = ?`,
		strings.ToUpper(p.Code), p.Name, p.Description, p.ClientName, p.Status, p.StartDate, p.TargetEndDate, p.InitialBudget, p.DefaultHourlyRate, p.Currency, p.OwnerUserID, p.RequireTOTP, p.UpdatedAt, p.ID,
	)
	return err
}

func (s *Store) DeleteProject(id string) error {
	_, _ = s.exec(`DELETE FROM project_members WHERE project_id = ?`, id)
	_, _ = s.exec(`DELETE FROM time_logs WHERE ticket_id IN (SELECT id FROM tickets WHERE project_id = ?)`, id)
	_, _ = s.exec(`DELETE FROM tickets WHERE project_id = ?`, id)
	_, _ = s.exec(`DELETE FROM wbs_dependencies WHERE project_id = ?`, id)
	_, _ = s.exec(`DELETE FROM wbs_nodes WHERE project_id = ?`, id)
	_, _ = s.exec(`DELETE FROM doc_pages WHERE project_id = ?`, id)
	_, err := s.exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// GetTOTPRequiredProjectIDs retourne la liste des ID de projets qui exigent la double authentification 2FA TOTP.
func (s *Store) GetTOTPRequiredProjectIDs() ([]string, error) {
	rows, err := s.query(`SELECT id FROM projects WHERE require_totp = ? ORDER BY created_at ASC`, true)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// --- Gestion des Membres & Permissions Fines d'un Projet (project_members) ---

func (s *Store) GetProjectMembers(projectID string) ([]models.ProjectMember, error) {
	rows, err := s.query(`
		SELECT pm.id, pm.project_id, pm.user_id,
		       COALESCE(NULLIF(pm.user_email, ''), u.email, ''),
		       COALESCE(NULLIF(pm.user_name, ''), u.display_name, u.username, ''),
		       pm.project_role,
		       pm.can_read, pm.can_write_tickets, pm.can_delete_tickets, pm.can_execute,
		       pm.can_edit_gantt, pm.can_edit_wiki, pm.can_view_finance, pm.can_manage_members,
		       pm.created_at
		FROM project_members pm
		LEFT JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = ?
		ORDER BY CASE pm.project_role
			WHEN 'owner' THEN 1
			WHEN 'manager' THEN 2
			WHEN 'contributor' THEN 3
			WHEN 'executor' THEN 4
			ELSE 5
		END, pm.created_at ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.ProjectMember
	for rows.Next() {
		var pm models.ProjectMember
		if err := rows.Scan(
			&pm.ID, &pm.ProjectID, &pm.UserID, &pm.UserEmail, &pm.UserName, &pm.ProjectRole,
			&pm.CanRead, &pm.CanWriteTickets, &pm.CanDeleteTickets, &pm.CanExecute,
			&pm.CanEditGantt, &pm.CanEditWiki, &pm.CanViewFinance, &pm.CanManageMembers,
			&pm.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, pm)
	}
	return list, nil
}

func (s *Store) SaveProjectMember(pm *models.ProjectMember) error {
	if pm.ID == "" {
		pm.ID = NewID("pm")
	}
	if pm.CreatedAt.IsZero() {
		pm.CreatedAt = time.Now()
	}
	if pm.ProjectRole == "" {
		pm.ProjectRole = models.ProjectRoleContributor
	}
	// Si le rôle est 'owner' (Maître du Projet), toutes les permissions sont garanties actives
	if pm.ProjectRole == models.ProjectRoleOwner {
		pm.CanRead = true
		pm.CanWriteTickets = true
		pm.CanDeleteTickets = true
		pm.CanExecute = true
		pm.CanEditGantt = true
		pm.CanEditWiki = true
		pm.CanViewFinance = true
		pm.CanManageMembers = true
	}
	if pm.UserEmail == "" || pm.UserName == "" {
		if u, err := s.GetUserByID(pm.UserID); err == nil && u != nil {
			if pm.UserEmail == "" {
				pm.UserEmail = u.Email
			}
			if pm.UserName == "" {
				pm.UserName = u.DisplayName
			}
		}
	}

	_, err := s.exec(`
		INSERT INTO project_members (
			id, project_id, user_id, user_email, user_name, project_role,
			can_read, can_write_tickets, can_delete_tickets, can_execute,
			can_edit_gantt, can_edit_wiki, can_view_finance, can_manage_members, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, user_id) DO UPDATE SET
			user_email = excluded.user_email,
			user_name = excluded.user_name,
			project_role = excluded.project_role,
			can_read = excluded.can_read,
			can_write_tickets = excluded.can_write_tickets,
			can_delete_tickets = excluded.can_delete_tickets,
			can_execute = excluded.can_execute,
			can_edit_gantt = excluded.can_edit_gantt,
			can_edit_wiki = excluded.can_edit_wiki,
			can_view_finance = excluded.can_view_finance,
			can_manage_members = excluded.can_manage_members`,
		pm.ID, pm.ProjectID, pm.UserID, strings.ToLower(pm.UserEmail), pm.UserName, pm.ProjectRole,
		pm.CanRead, pm.CanWriteTickets, pm.CanDeleteTickets, pm.CanExecute,
		pm.CanEditGantt, pm.CanEditWiki, pm.CanViewFinance, pm.CanManageMembers, pm.CreatedAt,
	)
	return err
}

func (s *Store) DeleteProjectMember(projectID, userID string) error {
	_, err := s.exec(`DELETE FROM project_members WHERE project_id = ? AND (user_id = ? OR id = ?)`, projectID, userID, userID)
	return err
}

// ResolveProjectPermissions calcule les permissions effectives d'un utilisateur sur un projet donné.
func (s *Store) ResolveProjectPermissions(u *models.User, proj *models.Project) models.ProjectMember {
	if u == nil || !u.IsActive || proj == nil {
		return models.PresetPermissionsForRole(models.ProjectRoleViewer)
	}

	// 1. Les SuperAdmins / Admins SaaS ou le Maître principal du projet (OwnerUserID) ont tous les droits
	if u.Role == models.RoleSuperAdmin || u.Role == models.RoleAdmin || (proj.OwnerUserID != "" && proj.OwnerUserID == u.ID) {
		perm := models.PresetPermissionsForRole(models.ProjectRoleOwner)
		perm.ProjectID = proj.ID
		perm.UserID = u.ID
		perm.UserEmail = u.Email
		perm.UserName = u.DisplayName
		return perm
	}

	// 2. Recherche d'une affectation explicite dans project_members (par user_id ou user_email)
	members, _ := s.GetProjectMembers(proj.ID)
	for _, m := range members {
		if m.UserID == u.ID || (m.UserEmail != "" && strings.EqualFold(m.UserEmail, u.Email)) {
			return m
		}
	}

	// 3. Repli selon le rôle global de l'utilisateur dans le Workspace
	var fallback models.ProjectMember
	switch u.Role {
	case models.RoleManager:
		fallback = models.PresetPermissionsForRole(models.ProjectRoleManager)
	case models.RoleMember:
		// Si le projet a déjà des membres explicites et que ce collaborateur n'en fait pas partie,
		// on lui donne par défaut les droits de Contributeur s'il n'y a aucun membre configuré, ou Lecteur sinon.
		if len(members) == 0 {
			fallback = models.PresetPermissionsForRole(models.ProjectRoleContributor)
		} else {
			fallback = models.PresetPermissionsForRole(models.ProjectRoleViewer)
		}
	default:
		fallback = models.PresetPermissionsForRole(models.ProjectRoleViewer)
	}
	fallback.ProjectID = proj.ID
	fallback.UserID = u.ID
	fallback.UserEmail = u.Email
	fallback.UserName = u.DisplayName
	return fallback
}

// Helpers de résolution d'ID de projet à partir d'une entité enfant (WBS, Dépendance, Page Wiki)
func (s *Store) GetProjectIDByWBSNode(nodeID string) string {
	var pid string
	_ = s.queryRow(`SELECT project_id FROM wbs_nodes WHERE id = ?`, nodeID).Scan(&pid)
	return pid
}

func (s *Store) GetProjectIDByDependency(depID string) string {
	var pid string
	_ = s.queryRow(`SELECT project_id FROM wbs_dependencies WHERE id = ?`, depID).Scan(&pid)
	return pid
}

func (s *Store) GetProjectIDByDocPage(docID string) string {
	var pid string
	_ = s.queryRow(`SELECT project_id FROM doc_pages WHERE id = ?`, docID).Scan(&pid)
	return pid
}

// --- WBS / Gantt ---

func (s *Store) GetWBSNodes(projectID string) ([]*models.WBSNode, error) {
	rows, err := s.query(`
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

func (s *Store) SaveWBSNode(n *models.WBSNode) error {
	if n.ID == "" {
		n.ID = NewID("wbs")
	}
	_, err := s.exec(`
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

func (s *Store) DeleteWBSNode(id string) error {
	_, _ = s.exec(`UPDATE tickets SET wbs_node_id = '' WHERE wbs_node_id = ?`, id)
	_, _ = s.exec(`UPDATE wbs_nodes SET parent_id = '' WHERE parent_id = ?`, id)
	_, _ = s.exec(`DELETE FROM wbs_dependencies WHERE predecessor_id = ? OR successor_id = ?`, id, id)
	_, err := s.exec(`DELETE FROM wbs_nodes WHERE id = ?`, id)
	return err
}

func (s *Store) GetWBSDependencies(projectID string) ([]models.WBSDependency, error) {
	rows, err := s.query(`
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

func (s *Store) CreateWBSDependency(d *models.WBSDependency) error {
	if d.ID == "" {
		d.ID = NewID("dep")
	}
	if d.DepType == "" {
		d.DepType = "FS"
	}
	_, err := s.exec(`
		INSERT INTO wbs_dependencies (id, project_id, predecessor_id, successor_id, dep_type, lag_days)
		VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.ProjectID, d.PredecessorID, d.SuccessorID, d.DepType, d.LagDays,
	)
	return err
}

func (s *Store) DeleteWBSDependency(id string) error {
	_, err := s.exec(`DELETE FROM wbs_dependencies WHERE id = ?`, id)
	return err
}

// --- Tickets, Sous-tickets & Pointages ---

func (s *Store) GetTickets(projectID string) ([]*models.Ticket, error) {
	rows, err := s.query(`
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

func (s *Store) GetTicket(id string) (*models.Ticket, error) {
	t := &models.Ticket{}
	err := s.queryRow(`
		SELECT id, project_id, wbs_node_id, parent_ticket_id, ticket_key, title, description, ticket_type, status, priority, assignee_id, estimated_hours, logged_hours, remaining_hours, fixed_cost, due_date, tags, created_at, updated_at
		FROM tickets WHERE id = ?`, id).Scan(
		&t.ID, &t.ProjectID, &t.WBSNodeID, &t.ParentTicketID, &t.TicketKey, &t.Title, &t.Description, &t.TicketType, &t.Status, &t.Priority, &t.AssigneeID, &t.EstimatedHours, &t.LoggedHours, &t.RemainingHours, &t.FixedCost, &t.DueDate, &t.Tags, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return t, nil
}

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
	if t.ParentTicketID == t.ID {
		t.ParentTicketID = ""
	}
	if t.TicketKey == "" {
		var cnt int
		_ = s.queryRow(`SELECT COUNT(*) FROM tickets WHERE project_id = ?`, t.ProjectID).Scan(&cnt)
		var pCode string
		_ = s.queryRow(`SELECT code FROM projects WHERE id = ?`, t.ProjectID).Scan(&pCode)
		if pCode == "" {
			pCode = "PRJ"
		}
		t.TicketKey = fmt.Sprintf("%s-%d", pCode, cnt+1)
	}

	_, err := s.exec(`
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

func (s *Store) DeleteTicket(id string) error {
	_, _ = s.exec(`UPDATE tickets SET parent_ticket_id = '' WHERE parent_ticket_id = ?`, id)
	_, _ = s.exec(`DELETE FROM time_logs WHERE ticket_id = ?`, id)
	_, err := s.exec(`DELETE FROM tickets WHERE id = ?`, id)
	return err
}

func (s *Store) GetTimeLogsByProject(projectID string) (map[string][]models.TimeLog, error) {
	rows, err := s.query(`
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

func (s *Store) AddTimeLog(l *models.TimeLog, newRemainingHours *float64) error {
	if l.ID == "" {
		l.ID = NewID("log")
	}
	l.CreatedAt = time.Now()
	if l.WorkDate == "" {
		l.WorkDate = l.CreatedAt.Format("2006-01-02")
	}
	if l.HourlyRateSnapshot <= 0 && l.MemberID != "" {
		_ = s.queryRow(`SELECT hourly_rate FROM members WHERE id = ?`, l.MemberID).Scan(&l.HourlyRateSnapshot)
	}
	if l.HourlyRateSnapshot <= 0 {
		l.HourlyRateSnapshot = 65.0
	}

	_, err := s.exec(`
		INSERT INTO time_logs (id, ticket_id, member_id, hours, hourly_rate_snapshot, work_date, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.TicketID, l.MemberID, l.Hours, l.HourlyRateSnapshot, l.WorkDate, l.Comment, l.CreatedAt,
	)
	if err != nil {
		return err
	}

	var totalLogged float64
	_ = s.queryRow(`SELECT COALESCE(SUM(hours), 0) FROM time_logs WHERE ticket_id = ?`, l.TicketID).Scan(&totalLogged)

	if newRemainingHours != nil {
		_, err = s.exec(`UPDATE tickets SET logged_hours = ?, remaining_hours = ?, updated_at = ? WHERE id = ?`,
			totalLogged, *newRemainingHours, time.Now(), l.TicketID)
	} else {
		var currRem float64
		_ = s.queryRow(`SELECT remaining_hours FROM tickets WHERE id = ?`, l.TicketID).Scan(&currRem)
		nextRem := currRem - l.Hours
		if nextRem < 0 {
			nextRem = 0
		}
		_, err = s.exec(`UPDATE tickets SET logged_hours = ?, remaining_hours = ?, updated_at = ? WHERE id = ?`,
			totalLogged, nextRem, time.Now(), l.TicketID)
	}
	return err
}

// --- Documentation (Wiki) ---

func (s *Store) GetDocPages(projectID string) ([]*models.DocPage, error) {
	rows, err := s.query(`
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

func (s *Store) SaveDocPage(d *models.DocPage) error {
	d.UpdatedAt = time.Now()
	if d.ID == "" {
		d.ID = NewID("doc")
	}
	_, err := s.exec(`
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

func (s *Store) DeleteDocPage(id string) error {
	_, err := s.exec(`DELETE FROM doc_pages WHERE id = ?`, id)
	return err
}
