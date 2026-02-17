package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Reminder represents a scheduled reminder
type Reminder struct {
	ID       string
	Channel  string
	UserID   string
	Message  string
	FireAt   time.Time
	Created  time.Time
}

// String returns a human-readable representation of the reminder
func (r *Reminder) String() string {
	shortID := r.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	remaining := time.Until(r.FireAt)
	if remaining < 0 {
		return fmt.Sprintf("⏰ [%s] %s (expired)", shortID, r.Message)
	}
	return fmt.Sprintf("⏰ [%s] in %s: %s", shortID, formatDuration(remaining), r.Message)
}

// ReminderHandler is called when a reminder fires
type ReminderHandler func(r *Reminder)

// Scheduler manages reminders
type Scheduler struct {
	reminders    map[string]*Reminder
	handler      ReminderHandler
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	tickInterval time.Duration
}

// New creates a new scheduler
func New() *Scheduler {
	return &Scheduler{
		reminders:    make(map[string]*Reminder),
		stopCh:       make(chan struct{}),
		tickInterval: 1 * time.Second,
	}
}

// NewWithInterval creates a scheduler with a custom tick interval (for testing)
func NewWithInterval(interval time.Duration) *Scheduler {
	return &Scheduler{
		reminders:    make(map[string]*Reminder),
		stopCh:       make(chan struct{}),
		tickInterval: interval,
	}
}

// SetHandler sets the function to call when reminders fire
func (s *Scheduler) SetHandler(h ReminderHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// Start begins the scheduler loop
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.loop()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
}

// loop checks for due reminders
func (s *Scheduler) loop() {
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.checkDue(now)
		}
	}
}

// checkDue fires any due reminders
func (s *Scheduler) checkDue(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, r := range s.reminders {
		if now.After(r.FireAt) || now.Equal(r.FireAt) {
			// Fire the reminder
			if s.handler != nil {
				// Call handler outside of lock to avoid deadlock
				reminder := r
				go s.handler(reminder)
			}
			// Remove the reminder
			delete(s.reminders, id)
		}
	}
}

// AddReminder schedules a new reminder
func (s *Scheduler) AddReminder(channel, userID string, in time.Duration, message string) (string, error) {
	if message == "" {
		return "", fmt.Errorf("message cannot be empty")
	}
	if in <= 0 {
		return "", fmt.Errorf("duration must be positive")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	r := &Reminder{
		ID:      id,
		Channel: channel,
		UserID:  userID,
		Message: message,
		FireAt:  time.Now().Add(in),
		Created: time.Now(),
	}

	s.reminders[id] = r
	return id, nil
}

// Cancel removes a reminder by ID
func (s *Scheduler) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.reminders[id]; exists {
		delete(s.reminders, id)
		return true
	}
	return false
}

// List returns all reminders for a user on a channel
func (s *Scheduler) List(channel, userID string) []*Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Reminder
	for _, r := range s.reminders {
		if r.Channel == channel && r.UserID == userID {
			result = append(result, r)
		}
	}
	return result
}

// ParseDuration parses a duration string with support for d (days) and w (weeks)
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Handle weeks
	if strings.HasSuffix(s, "w") {
		weeks, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid weeks: %s", s)
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	// Handle days
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid days: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Standard Go duration
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}
	return d, nil
}

// ParseRemindCommand parses "in <duration> <message>" format
func ParseRemindCommand(args string) (durationStr, message string, err error) {
	args = strings.TrimSpace(args)

	// Match: "in <duration> <message>"
	re := regexp.MustCompile(`^in\s+(\S+)\s+(.+)$`)
	matches := re.FindStringSubmatch(args)

	if len(matches) != 3 {
		return "", "", fmt.Errorf("invalid format: use 'in <duration> <message>'")
	}

	durationStr = matches[1]
	message = strings.TrimSpace(matches[2])

	// Validate duration
	if _, err := ParseDuration(durationStr); err != nil {
		return "", "", fmt.Errorf("invalid duration: %s", durationStr)
	}

	if message == "" {
		return "", "", fmt.Errorf("message cannot be empty")
	}

	return durationStr, message, nil
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%dh%dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return fmt.Sprintf("%dd", days)
}
