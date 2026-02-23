// Package heartbeat provides a background service for sending periodic
// proactive messages to active users.
package heartbeat

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/internal/logger"
)

const (
	// DefaultIntervalMinutes is the default heartbeat interval
	DefaultIntervalMinutes = 60
)

// Config holds heartbeat configuration
type Config struct {
	Enabled         bool `yaml:"enabled"`
	IntervalMinutes int  `yaml:"intervalMinutes"`
}

// DefaultConfig returns the default heartbeat configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:         false,
		IntervalMinutes: DefaultIntervalMinutes,
	}
}

// User represents an active user to send heartbeats to
type User struct {
	Channel string
	UserID  int64
}

// Callback is called when a heartbeat should be sent
type Callback func(channel string, userID int64, message string)

// Service manages periodic heartbeat messages
type Service struct {
	cfg           *Config
	workspacePath string
	users         map[string]*User // keyed by channel:userID
	callback      Callback
	stopChan      chan struct{}
	running       bool
	mu            sync.RWMutex
}

// New creates a new heartbeat service
func New(cfg *Config, workspacePath string) *Service {
	return &Service{
		cfg:           cfg,
		workspacePath: workspacePath,
		users:         make(map[string]*User),
	}
}

// SetCallback sets the function to call when sending heartbeats
func (s *Service) SetCallback(cb Callback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = cb
}

// RegisterUser adds a user to receive heartbeats
func (s *Service) RegisterUser(channel string, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%d", channel, userID)
	s.users[key] = &User{
		Channel: channel,
		UserID:  userID,
	}
}

// UnregisterUser removes a user from receiving heartbeats
func (s *Service) UnregisterUser(channel string, userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%d", channel, userID)
	delete(s.users, key)
}

// ListActiveUsers returns all registered users
func (s *Service) ListActiveUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	return users
}

// Start begins the heartbeat loop
func (s *Service) Start() {
	s.mu.Lock()
	if !s.cfg.Enabled {
		s.mu.Unlock()
		return
	}

	if s.running {
		s.mu.Unlock()
		return
	}

	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	interval := time.Duration(s.cfg.IntervalMinutes) * time.Minute
	logger.Info("💓 Heartbeat service started (interval: %v)", interval)

	go s.loop(interval)
}

// Stop stops the heartbeat service
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopChan)
	logger.Info("💓 Heartbeat service stopped")
}

// IsRunning returns whether the service is running
func (s *Service) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// loop is the main heartbeat loop
func (s *Service) loop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.sendHeartbeats()
		}
	}
}

// sendHeartbeats sends heartbeat messages to all registered users
func (s *Service) sendHeartbeats() {
	s.mu.RLock()
	callback := s.callback
	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	s.mu.RUnlock()

	if callback == nil || len(users) == 0 {
		return
	}

	message := s.BuildHeartbeatMessage()
	if message == "" {
		return
	}

	for _, user := range users {
		callback(user.Channel, user.UserID, message)
	}
}

// LoadHeartbeatTasks reads HEARTBEAT.md from the workspace
func (s *Service) LoadHeartbeatTasks() string {
	if s.workspacePath == "" {
		return ""
	}

	path := filepath.Join(s.workspacePath, "HEARTBEAT.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return string(data)
}

// BuildHeartbeatMessage creates the heartbeat message
func (s *Service) BuildHeartbeatMessage() string {
	// Load custom tasks from HEARTBEAT.md
	tasks := s.LoadHeartbeatTasks()

	// If no custom tasks, just use time-based greeting
	if tasks == "" {
		hour := time.Now().Hour()
		return s.getTimeGreeting(hour)
	}

	// Combine greeting with tasks
	hour := time.Now().Hour()
	greeting := s.getTimeGreeting(hour)

	return fmt.Sprintf("%s\n\n---\n\n%s", greeting, tasks)
}

// getTimeGreeting returns a time-appropriate greeting
func (s *Service) getTimeGreeting(hour int) string {
	switch {
	case hour >= 5 && hour < 12:
		return "☀️ Good morning! Hope you're having a great start to your day."
	case hour >= 12 && hour < 17:
		return "🌤️ Good afternoon! How's your day going?"
	default:
		return "🌙 Good evening! Winding down for the day?"
	}
}

// ForceHeartbeat triggers an immediate heartbeat (for testing/debug)
func (s *Service) ForceHeartbeat() {
	s.sendHeartbeats()
}
