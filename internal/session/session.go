package session

import (
	"log"
	"sync"
	"time"

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

// Session represents a conversation session with message history
type Session struct {
	Key        string
	Messages   []types.Message
	CreatedAt  time.Time
	UpdatedAt  time.Time
	MaxHistory int
	Model      string // Per-session model override
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

	for _, key := range keys {
		messages, model, err := p.Load(key)
		if err != nil {
			log.Printf("⚠️  Failed to load session %s: %v", key, err)
			continue
		}

		sess := &Session{
			Key:        key,
			Messages:   messages,
			Model:      model,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			MaxHistory: DefaultMaxHistory,
		}

		s.mu.Lock()
		s.sessions[key] = sess
		s.mu.Unlock()
	}

	if len(keys) > 0 {
		log.Printf("📂 Loaded %d sessions from database", len(keys))
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
			log.Printf("⚠️  Failed to delete session from DB: %v", err)
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
		if err := persister.Save(sess.Key, messages, model); err != nil {
			log.Printf("⚠️  Failed to persist session: %v", err)
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
	if err := persister.Save(key, messages, model); err != nil {
		log.Printf("⚠️  Failed to persist session: %v", err)
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
			log.Printf("⚠️  Failed to delete session from DB: %v", err)
		}
	}
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
