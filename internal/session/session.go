package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/internal/logger"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	// DefaultMaxHistory is the default maximum number of messages to keep per session
	DefaultMaxHistory = 50
)

// Persister is the interface for session persistence
type Persister interface {
	Save(key string, messages []types.Message, model string) error
	Load(key string) ([]types.Message, string, error)
	Delete(key string) error
	ListKeys() ([]string, error)
}

// PersisterWithProfile extends Persister with profile support
type PersisterWithProfile interface {
	Persister
	SaveWithProfile(key string, messages []types.Message, model, profile string) error
	LoadWithProfile(key string) ([]types.Message, string, string, error)
}

// Session represents a conversation session with message history
type Session struct {
	Key        string
	Messages   []types.Message
	CreatedAt  time.Time
	UpdatedAt  time.Time
	MaxHistory int
	Model      string // Per-session model override
	TTSEnabled *bool  // Per-session TTS toggle (nil = use global config)
	Profile    string // Per-session personality profile name
	mu         sync.Mutex
}

// Store manages conversation sessions in memory
type Store struct {
	sessions  map[string]*Session
	persister Persister
	mu        sync.RWMutex
}

// NewStore creates a new session store
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
	}
}

// SetPersister sets the persistence backend and loads existing sessions
func (s *Store) SetPersister(p Persister) error {
	s.mu.Lock()
	s.persister = p
	s.mu.Unlock()

	// Load all existing sessions from persistence
	keys, err := p.ListKeys()
	if err != nil {
		return err
	}

	// Check if persister supports profile loading
	pWithProfile, hasProfile := p.(PersisterWithProfile)

	for _, key := range keys {
		var messages []types.Message
		var model, profile string

		if hasProfile {
			messages, model, profile, err = pWithProfile.LoadWithProfile(key)
		} else {
			messages, model, err = p.Load(key)
		}

		if err != nil {
			logger.Warn("⚠️  Failed to load session %s: %v", key, err)
			continue
		}

		sess := &Session{
			Key:        key,
			Messages:   messages,
			Model:      model,
			Profile:    profile,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			MaxHistory: DefaultMaxHistory,
		}

		s.mu.Lock()
		s.sessions[key] = sess
		s.mu.Unlock()
	}

	if len(keys) > 0 {
		logger.Info("📂 Loaded %d sessions from database", len(keys))
	}

	return nil
}

// SessionKey generates a unique key for a channel+user combination
func SessionKey(channel, userID string) string {
	return channel + ":" + userID
}

// GetOrCreate retrieves an existing session or creates a new one
func (s *Store) GetOrCreate(channel, userID string) *Session {
	key := SessionKey(channel, userID)

	s.mu.RLock()
	sess, exists := s.sessions[key]
	s.mu.RUnlock()

	if exists {
		return sess
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if sess, exists = s.sessions[key]; exists {
		return sess
	}

	sess = &Session{
		Key:        key,
		Messages:   make([]types.Message, 0),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		MaxHistory: DefaultMaxHistory,
	}
	s.sessions[key] = sess

	return sess
}

// Get retrieves a session without creating one
func (s *Store) Get(channel, userID string) (*Session, bool) {
	key := SessionKey(channel, userID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, exists := s.sessions[key]
	return sess, exists
}

// Delete removes a session from the store
func (s *Store) Delete(channel, userID string) {
	key := SessionKey(channel, userID)

	s.mu.Lock()
	persister := s.persister
	delete(s.sessions, key)
	s.mu.Unlock()

	// Delete from persistence
	if persister != nil {
		if err := persister.Delete(key); err != nil {
			logger.Warn("⚠️  Failed to delete session from DB: %v", err)
		}
	}
}

// AddMessage adds a message to the session history
func (sess *Session) AddMessage(msg types.Message) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Messages = append(sess.Messages, msg)
	sess.UpdatedAt = time.Now()

	// Trim to MaxHistory if set
	if sess.MaxHistory > 0 && len(sess.Messages) > sess.MaxHistory {
		excess := len(sess.Messages) - sess.MaxHistory
		sess.Messages = sess.Messages[excess:]
	}
}

// AddMessageAndPersist adds a message and persists the session
func (s *Store) AddMessageAndPersist(channel, userID string, msg types.Message) {
	sess := s.GetOrCreate(channel, userID)
	sess.AddMessage(msg)

	s.mu.RLock()
	persister := s.persister
	s.mu.RUnlock()

	if persister != nil {
		messages := sess.GetAllMessages()
		model := sess.GetModel()
		profile := sess.GetProfile()

		// Use SaveWithProfile if available
		if pWithProfile, ok := persister.(PersisterWithProfile); ok {
			if err := pWithProfile.SaveWithProfile(sess.Key, messages, model, profile); err != nil {
				logger.Warn("⚠️  Failed to persist session: %v", err)
			}
		} else {
			if err := persister.Save(sess.Key, messages, model); err != nil {
				logger.Warn("⚠️  Failed to persist session: %v", err)
			}
		}
	}
}

// Persist saves the session to the persistence backend
func (s *Store) Persist(channel, userID string) {
	s.mu.RLock()
	persister := s.persister
	key := SessionKey(channel, userID)
	sess, exists := s.sessions[key]
	s.mu.RUnlock()

	if !exists || persister == nil {
		return
	}

	messages := sess.GetAllMessages()
	model := sess.GetModel()
	profile := sess.GetProfile()

	// Use SaveWithProfile if available
	if pWithProfile, ok := persister.(PersisterWithProfile); ok {
		if err := pWithProfile.SaveWithProfile(key, messages, model, profile); err != nil {
			logger.Warn("⚠️  Failed to persist session: %v", err)
		}
	} else {
		if err := persister.Save(key, messages, model); err != nil {
			logger.Warn("⚠️  Failed to persist session: %v", err)
		}
	}
}

// GetHistory returns the last n messages from the session
func (sess *Session) GetHistory(n int) []types.Message {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	if n >= len(sess.Messages) {
		// Return a copy to avoid race conditions
		result := make([]types.Message, len(sess.Messages))
		copy(result, sess.Messages)
		return result
	}

	start := len(sess.Messages) - n
	result := make([]types.Message, n)
	copy(result, sess.Messages[start:])
	return result
}

// GetAllMessages returns all messages in the session
func (sess *Session) GetAllMessages() []types.Message {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	result := make([]types.Message, len(sess.Messages))
	copy(result, sess.Messages)
	return result
}

// ReplaceHistory replaces all messages in the session (used by compaction)
func (sess *Session) ReplaceHistory(messages []types.Message) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Messages = make([]types.Message, len(messages))
	copy(sess.Messages, messages)
	sess.UpdatedAt = time.Now()
}

// Clear removes all messages from the session and resets model
func (sess *Session) Clear() {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Messages = make([]types.Message, 0)
	sess.Model = ""
	sess.UpdatedAt = time.Now()
}

// ClearAndPersist clears the session and removes it from persistence
func (s *Store) ClearAndPersist(channel, userID string) {
	key := SessionKey(channel, userID)

	s.mu.RLock()
	sess, exists := s.sessions[key]
	persister := s.persister
	s.mu.RUnlock()

	if exists {
		sess.Clear()
	}

	// Delete from persistence (clear = remove from DB)
	if persister != nil {
		if err := persister.Delete(key); err != nil {
			logger.Warn("⚠️  Failed to delete session from DB: %v", err)
		}
	}
}

// ClearAll removes all sessions from memory (does NOT clear database)
func (s *Store) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = make(map[string]*Session)
}

// SetModel sets a per-session model override
func (sess *Session) SetModel(model string) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Model = model
	sess.UpdatedAt = time.Now()
}

// GetModel returns the session's model (or empty for default)
func (sess *Session) GetModel() string {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	return sess.Model
}

// SetTTS sets the per-session TTS preference
func (sess *Session) SetTTS(enabled bool) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.TTSEnabled = &enabled
	sess.UpdatedAt = time.Now()
}

// GetTTS returns the session's TTS preference (nil = use global)
func (sess *Session) GetTTS() *bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	return sess.TTSEnabled
}

// SetProfile sets the per-session personality profile
func (sess *Session) SetProfile(profile string) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	sess.Profile = profile
	sess.UpdatedAt = time.Now()
}

// GetProfile returns the session's active profile (empty = default)
func (sess *Session) GetProfile() string {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	return sess.Profile
}

// Len returns the number of messages in the session
func (sess *Session) Len() int {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	return len(sess.Messages)
}

// supportedModels is the list of known valid Anthropic models
var supportedModels = []string{
	"claude-sonnet-4-20250514",
	"claude-opus-4-20250514",
	"claude-3-5-sonnet-20241022",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-3-haiku-20240307",
}

// UserSessionEntry tracks a user's session
type UserSessionEntry struct {
	SessionID string    // Full session key
	Name      string    // User-friendly name
	CreatedAt time.Time
	UpdatedAt time.Time
	IsActive  bool
}

// ValidateModel checks if a model name is supported
func ValidateModel(model string) bool {
	if model == "" {
		return false
	}
	for _, m := range supportedModels {
		if m == model {
			return true
		}
	}
	return false
}

// SupportedModels returns the list of supported models
func SupportedModels() []string {
	return supportedModels
}

// Count returns the total number of active sessions
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// GetRecent returns the most recently updated sessions (up to limit)
func (s *Store) GetRecent(limit int) []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}

	// Sort by UpdatedAt descending
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].UpdatedAt.After(sessions[i].UpdatedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions
}

// Channel returns the channel part of the session key
func (sess *Session) Channel() string {
	parts := splitKey(sess.Key)
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// UserID returns the user ID part of the session key
func (sess *Session) UserID() string {
	parts := splitKey(sess.Key)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// splitKey splits a session key into channel and user ID
func splitKey(key string) []string {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}

// Fork creates a copy of the session with a new ID
func (s *Store) Fork(channel, userID string, newSessionID string) (*Session, error) {
	oldKey := SessionKey(channel, userID)
	newKey := SessionKey(channel, userID) + ":" + newSessionID

	s.mu.RLock()
	oldSess, exists := s.sessions[oldKey]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	// Create a copy
	oldSess.mu.Lock()
	newMessages := make([]types.Message, len(oldSess.Messages))
	copy(newMessages, oldSess.Messages)
	model := oldSess.Model
	profile := oldSess.Profile
	ttsEnabled := oldSess.TTSEnabled
	oldSess.mu.Unlock()

	newSess := &Session{
		Key:        newKey,
		Messages:   newMessages,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		MaxHistory: DefaultMaxHistory,
		Model:      model,
		Profile:    profile,
		TTSEnabled: ttsEnabled,
	}

	s.mu.Lock()
	s.sessions[newKey] = newSess
	s.mu.Unlock()

	return newSess, nil
}

// ListUserSessions returns all sessions for a user on a channel
func (s *Store) ListUserSessions(channel, userID string) []UserSessionEntry {
	baseKey := SessionKey(channel, userID)
	var entries []UserSessionEntry

	s.mu.RLock()
	defer s.mu.RUnlock()

	for key, sess := range s.sessions {
		// Match base key or forked sessions (baseKey:forkID)
		if key == baseKey || (len(key) > len(baseKey)+1 && key[:len(baseKey)+1] == baseKey+":") {
			name := "main"
			if key != baseKey {
				// Extract fork name
				name = key[len(baseKey)+1:]
			}

			entries = append(entries, UserSessionEntry{
				SessionID: key,
				Name:      name,
				CreatedAt: sess.CreatedAt,
				UpdatedAt: sess.UpdatedAt,
				IsActive:  true, // For now all loaded sessions are active
			})
		}
	}

	return entries
}

// SwitchSession sets the active session for a user
// Returns the new session or error if not found
func (s *Store) SwitchSession(channel, userID, sessionName string) (*Session, error) {
	var targetKey string
	if sessionName == "main" || sessionName == "" {
		targetKey = SessionKey(channel, userID)
	} else {
		targetKey = SessionKey(channel, userID) + ":" + sessionName
	}

	s.mu.RLock()
	sess, exists := s.sessions[targetKey]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session '%s' not found", sessionName)
	}

	return sess, nil
}

// GetSession retrieves a specific session by full key
func (s *Store) GetSession(key string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, exists := s.sessions[key]
	return sess, exists
}
