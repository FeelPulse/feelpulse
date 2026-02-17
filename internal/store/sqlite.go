package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore persists sessions to a SQLite database
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store at the given path
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent write performance
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")

	// Create table if not exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			key TEXT PRIMARY KEY,
			messages TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	// Add profile column if not exists (migration)
	_, _ = db.Exec(`ALTER TABLE sessions ADD COLUMN profile TEXT NOT NULL DEFAULT ''`)

	return &SQLiteStore{db: db}, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Save persists a session to the database (upsert)
func (s *SQLiteStore) Save(key string, messages []types.Message, model string) error {
	return s.SaveWithProfile(key, messages, model, "")
}

// SaveWithProfile persists a session with model and profile to the database (upsert)
func (s *SQLiteStore) SaveWithProfile(key string, messages []types.Message, model, profile string) error {
	// Serialize messages to JSON
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	now := time.Now().Unix()

	// Upsert using INSERT OR REPLACE
	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO sessions (key, messages, model, profile, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, key, string(data), model, profile, now)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Load retrieves a session from the database
// Returns nil messages if session doesn't exist
func (s *SQLiteStore) Load(key string) ([]types.Message, string, error) {
	messages, model, _, err := s.LoadWithProfile(key)
	return messages, model, err
}

// LoadWithProfile retrieves a session with model and profile from the database
// Returns nil messages if session doesn't exist
func (s *SQLiteStore) LoadWithProfile(key string) ([]types.Message, string, string, error) {
	var messagesJSON string
	var model string
	var profile sql.NullString

	err := s.db.QueryRow(`
		SELECT messages, model, profile FROM sessions WHERE key = ?
	`, key).Scan(&messagesJSON, &model, &profile)

	if err == sql.ErrNoRows {
		return nil, "", "", nil
	}
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to load session: %w", err)
	}

	var messages []types.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return nil, "", "", fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	profileStr := ""
	if profile.Valid {
		profileStr = profile.String
	}

	return messages, model, profileStr, nil
}

// Delete removes a session from the database
func (s *SQLiteStore) Delete(key string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// ListKeys returns all session keys in the database
func (s *SQLiteStore) ListKeys() ([]string, error) {
	rows, err := s.db.Query(`SELECT key FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		keys = append(keys, key)
	}

	return keys, rows.Err()
}

// DefaultDBPath returns the default database path
func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".feelpulse", "sessions.db")
}

// === Reminder Persistence ===

// ReminderData represents a reminder for storage
type ReminderData struct {
	ID       string    `json:"id"`
	Channel  string    `json:"channel"`
	UserID   string    `json:"user_id"`
	Message  string    `json:"message"`
	FireAt   time.Time `json:"fire_at"`
	Created  time.Time `json:"created"`
}

// EnsureRemindersTable creates the reminders table if it doesn't exist
func (s *SQLiteStore) EnsureRemindersTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS reminders (
			id TEXT PRIMARY KEY,
			channel TEXT NOT NULL,
			user_id TEXT NOT NULL,
			message TEXT NOT NULL,
			fire_at INTEGER NOT NULL,
			created INTEGER NOT NULL
		)
	`)
	return err
}

// SaveReminder persists a reminder to the database
func (s *SQLiteStore) SaveReminder(r *ReminderData) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO reminders (id, channel, user_id, message, fire_at, created)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.ID, r.Channel, r.UserID, r.Message, r.FireAt.Unix(), r.Created.Unix())
	return err
}

// DeleteReminder removes a reminder from the database
func (s *SQLiteStore) DeleteReminder(id string) error {
	_, err := s.db.Exec(`DELETE FROM reminders WHERE id = ?`, id)
	return err
}

// LoadReminders retrieves all reminders from the database
func (s *SQLiteStore) LoadReminders() ([]*ReminderData, error) {
	rows, err := s.db.Query(`
		SELECT id, channel, user_id, message, fire_at, created 
		FROM reminders 
		ORDER BY fire_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reminders []*ReminderData
	for rows.Next() {
		var r ReminderData
		var fireAtUnix, createdUnix int64
		if err := rows.Scan(&r.ID, &r.Channel, &r.UserID, &r.Message, &fireAtUnix, &createdUnix); err != nil {
			return nil, err
		}
		r.FireAt = time.Unix(fireAtUnix, 0)
		r.Created = time.Unix(createdUnix, 0)
		reminders = append(reminders, &r)
	}
	return reminders, rows.Err()
}

// CleanExpiredReminders removes reminders that have already fired
func (s *SQLiteStore) CleanExpiredReminders() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM reminders WHERE fire_at < ?`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// === Sub-Agent Persistence ===

// SubAgentData represents a sub-agent for storage
type SubAgentData struct {
	ID               string    `json:"id"`
	Label            string    `json:"label"`
	Task             string    `json:"task"`
	SystemPrompt     string    `json:"system_prompt"`
	Status           string    `json:"status"`
	Result           string    `json:"result"`
	Error            string    `json:"error"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
	ParentSessionKey string    `json:"parent_session_key"`
}

// EnsureSubAgentsTable creates the sub_agents table if it doesn't exist
func (s *SQLiteStore) EnsureSubAgentsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sub_agents (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			task TEXT NOT NULL,
			system_prompt TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL,
			completed_at INTEGER NOT NULL DEFAULT 0,
			parent_session_key TEXT NOT NULL
		)
	`)
	return err
}

// SaveSubAgent persists a sub-agent to the database
func (s *SQLiteStore) SaveSubAgent(sa *SubAgentData) error {
	completedAt := int64(0)
	if !sa.CompletedAt.IsZero() {
		completedAt = sa.CompletedAt.Unix()
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO sub_agents (id, label, task, system_prompt, status, result, error, started_at, completed_at, parent_session_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sa.ID, sa.Label, sa.Task, sa.SystemPrompt, sa.Status, sa.Result, sa.Error, sa.StartedAt.Unix(), completedAt, sa.ParentSessionKey)
	return err
}

// DeleteSubAgent removes a sub-agent from the database
func (s *SQLiteStore) DeleteSubAgent(id string) error {
	_, err := s.db.Exec(`DELETE FROM sub_agents WHERE id = ?`, id)
	return err
}

// LoadSubAgent retrieves a sub-agent by ID
func (s *SQLiteStore) LoadSubAgent(id string) (*SubAgentData, error) {
	var sa SubAgentData
	var startedAtUnix, completedAtUnix int64

	err := s.db.QueryRow(`
		SELECT id, label, task, system_prompt, status, result, error, started_at, completed_at, parent_session_key
		FROM sub_agents WHERE id = ?
	`, id).Scan(&sa.ID, &sa.Label, &sa.Task, &sa.SystemPrompt, &sa.Status, &sa.Result, &sa.Error, &startedAtUnix, &completedAtUnix, &sa.ParentSessionKey)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	sa.StartedAt = time.Unix(startedAtUnix, 0)
	if completedAtUnix > 0 {
		sa.CompletedAt = time.Unix(completedAtUnix, 0)
	}

	return &sa, nil
}

// LoadAllSubAgents retrieves all sub-agents from the database
func (s *SQLiteStore) LoadAllSubAgents() ([]*SubAgentData, error) {
	rows, err := s.db.Query(`
		SELECT id, label, task, system_prompt, status, result, error, started_at, completed_at, parent_session_key
		FROM sub_agents 
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*SubAgentData
	for rows.Next() {
		var sa SubAgentData
		var startedAtUnix, completedAtUnix int64
		if err := rows.Scan(&sa.ID, &sa.Label, &sa.Task, &sa.SystemPrompt, &sa.Status, &sa.Result, &sa.Error, &startedAtUnix, &completedAtUnix, &sa.ParentSessionKey); err != nil {
			return nil, err
		}
		sa.StartedAt = time.Unix(startedAtUnix, 0)
		if completedAtUnix > 0 {
			sa.CompletedAt = time.Unix(completedAtUnix, 0)
		}
		agents = append(agents, &sa)
	}
	return agents, rows.Err()
}

// LoadPendingSubAgents retrieves sub-agents that were running when the server stopped
func (s *SQLiteStore) LoadPendingSubAgents() ([]*SubAgentData, error) {
	rows, err := s.db.Query(`
		SELECT id, label, task, system_prompt, status, result, error, started_at, completed_at, parent_session_key
		FROM sub_agents 
		WHERE status IN ('pending', 'running')
		ORDER BY started_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*SubAgentData
	for rows.Next() {
		var sa SubAgentData
		var startedAtUnix, completedAtUnix int64
		if err := rows.Scan(&sa.ID, &sa.Label, &sa.Task, &sa.SystemPrompt, &sa.Status, &sa.Result, &sa.Error, &startedAtUnix, &completedAtUnix, &sa.ParentSessionKey); err != nil {
			return nil, err
		}
		sa.StartedAt = time.Unix(startedAtUnix, 0)
		if completedAtUnix > 0 {
			sa.CompletedAt = time.Unix(completedAtUnix, 0)
		}
		agents = append(agents, &sa)
	}
	return agents, rows.Err()
}

// CleanOldSubAgents removes completed sub-agents older than maxAge
func (s *SQLiteStore) CleanOldSubAgents(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	result, err := s.db.Exec(`
		DELETE FROM sub_agents 
		WHERE status IN ('done', 'failed', 'canceled') 
		AND completed_at > 0 
		AND completed_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// === Pin Persistence ===

// PinData represents a pinned item for storage
type PinData struct {
	ID         string    `json:"id"`
	SessionKey string    `json:"session_key"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
}

// EnsurePinsTable creates the pins table if it doesn't exist
func (s *SQLiteStore) EnsurePinsTable() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pins (
			id TEXT PRIMARY KEY,
			session_key TEXT NOT NULL,
			text TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	// Index for fast lookup by session
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_pins_session ON pins(session_key)`)
	return nil
}

// SavePin persists a pin to the database
func (s *SQLiteStore) SavePin(p *PinData) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO pins (id, session_key, text, created_at)
		VALUES (?, ?, ?, ?)
	`, p.ID, p.SessionKey, p.Text, p.CreatedAt.Unix())
	return err
}

// DeletePin removes a pin from the database
func (s *SQLiteStore) DeletePin(id string) error {
	_, err := s.db.Exec(`DELETE FROM pins WHERE id = ?`, id)
	return err
}

// LoadPinsBySession retrieves all pins for a session
func (s *SQLiteStore) LoadPinsBySession(sessionKey string) ([]*PinData, error) {
	rows, err := s.db.Query(`
		SELECT id, session_key, text, created_at 
		FROM pins 
		WHERE session_key = ?
		ORDER BY created_at ASC
	`, sessionKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pins []*PinData
	for rows.Next() {
		var p PinData
		var createdAtUnix int64
		if err := rows.Scan(&p.ID, &p.SessionKey, &p.Text, &createdAtUnix); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(createdAtUnix, 0)
		pins = append(pins, &p)
	}
	return pins, rows.Err()
}

// LoadPin retrieves a single pin by ID
func (s *SQLiteStore) LoadPin(id string) (*PinData, error) {
	var p PinData
	var createdAtUnix int64

	err := s.db.QueryRow(`
		SELECT id, session_key, text, created_at FROM pins WHERE id = ?
	`, id).Scan(&p.ID, &p.SessionKey, &p.Text, &createdAtUnix)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	p.CreatedAt = time.Unix(createdAtUnix, 0)
	return &p, nil
}
