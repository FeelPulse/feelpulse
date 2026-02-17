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
