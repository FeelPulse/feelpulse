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
	// Serialize messages to JSON
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	now := time.Now().Unix()

	// Upsert using INSERT OR REPLACE
	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO sessions (key, messages, model, updated_at)
		VALUES (?, ?, ?, ?)
	`, key, string(data), model, now)

	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Load retrieves a session from the database
// Returns nil messages if session doesn't exist
func (s *SQLiteStore) Load(key string) ([]types.Message, string, error) {
	var messagesJSON string
	var model string

	err := s.db.QueryRow(`
		SELECT messages, model FROM sessions WHERE key = ?
	`, key).Scan(&messagesJSON, &model)

	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to load session: %w", err)
	}

	var messages []types.Message
	if err := json.Unmarshal([]byte(messagesJSON), &messages); err != nil {
		return nil, "", fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	return messages, model, nil
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
