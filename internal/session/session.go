package session

import (
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	// DefaultMaxHistory is the default maximum number of messages to keep per session
	DefaultMaxHistory = 50
)

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
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewStore creates a new session store
func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
	}
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
	defer s.mu.Unlock()

	delete(s.sessions, key)
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
