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

// ReminderPersister handles reminder persistence
type ReminderPersister interface {
	SaveReminder(r *ReminderData) error
	DeleteReminder(id string) error
	LoadReminders() ([]*ReminderData, error)
}

// ReminderData represents a reminder for storage
type ReminderData struct {
	ID       string
	Channel  string
	UserID   string
	Message  string
	FireAt   time.Time
	Created  time.Time
}

// Scheduler manages reminders
type Scheduler struct {
	reminders    map[string]*Reminder
	handler      ReminderHandler
	persister    ReminderPersister
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

// SetPersister sets the persistence backend and loads existing reminders
func (s *Scheduler) SetPersister(p ReminderPersister) error {
	s.mu.Lock()
	s.persister = p
	s.mu.Unlock()

	// Load existing reminders
	reminders, err := p.LoadReminders()
	if err != nil {
		return err
	}

	now := time.Now()
	loaded := 0
	for _, r := range reminders {
		// Skip expired reminders
		if r.FireAt.Before(now) {
			_ = p.DeleteReminder(r.ID)
			continue
		}

		s.mu.Lock()
		s.reminders[r.ID] = &Reminder{
			ID:      r.ID,
			Channel: r.Channel,
			UserID:  r.UserID,
			Message: r.Message,
			FireAt:  r.FireAt,
			Created: r.Created,
		}
		s.mu.Unlock()
		loaded++
	}

	if loaded > 0 {
		fmt.Printf("⏰ Loaded %d reminders from database\n", loaded)
	}

	return nil
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
	persister := s.persister
	var toFire []*Reminder
	var toDelete []string

	for id, r := range s.reminders {
		if now.After(r.FireAt) || now.Equal(r.FireAt) {
			toFire = append(toFire, r)
			toDelete = append(toDelete, id)
			delete(s.reminders, id)
		}
	}
	handler := s.handler
	s.mu.Unlock()

	// Fire reminders and delete from persistence outside lock
	for _, r := range toFire {
		if handler != nil {
			go handler(r)
		}
	}

	if persister != nil {
		for _, id := range toDelete {
			_ = persister.DeleteReminder(id)
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
	persister := s.persister
	s.mu.Unlock()

	id := uuid.New().String()
	now := time.Now()
	r := &Reminder{
		ID:      id,
		Channel: channel,
		UserID:  userID,
		Message: message,
		FireAt:  now.Add(in),
		Created: now,
	}

	s.mu.Lock()
	s.reminders[id] = r
	s.mu.Unlock()

	// Persist if available
	if persister != nil {
		_ = persister.SaveReminder(&ReminderData{
			ID:      r.ID,
			Channel: r.Channel,
			UserID:  r.UserID,
			Message: r.Message,
			FireAt:  r.FireAt,
			Created: r.Created,
		})
	}

	return id, nil
}

// Cancel removes a reminder by ID
func (s *Scheduler) Cancel(id string) bool {
	s.mu.Lock()
	_, exists := s.reminders[id]
	persister := s.persister
	if exists {
		delete(s.reminders, id)
	}
	s.mu.Unlock()

	if exists && persister != nil {
		_ = persister.DeleteReminder(id)
	}
	return exists
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

// ParseRemindCommand parses "in <duration> <message>" or "at <time> <message>" format
func ParseRemindCommand(args string) (durationStr, message string, err error) {
	args = strings.TrimSpace(args)

	// Match: "in <duration> <message>"
	reIn := regexp.MustCompile(`^in\s+(\S+)\s+(.+)$`)
	if matches := reIn.FindStringSubmatch(args); len(matches) == 3 {
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

	// Match: "at <HH:MM> <message>" or "at <HH:MM:SS> <message>"
	reAt := regexp.MustCompile(`^at\s+(\d{1,2}:\d{2}(?::\d{2})?)\s+(.+)$`)
	if matches := reAt.FindStringSubmatch(args); len(matches) == 3 {
		timeStr := matches[1]
		message = strings.TrimSpace(matches[2])

		// Parse the time and convert to duration
		dur, err := ParseAbsoluteTime(timeStr)
		if err != nil {
			return "", "", err
		}

		// Return as duration string (in seconds)
		durationStr = fmt.Sprintf("%ds", int(dur.Seconds()))

		if message == "" {
			return "", "", fmt.Errorf("message cannot be empty")
		}

		return durationStr, message, nil
	}

	return "", "", fmt.Errorf("invalid format: use 'in <duration> <message>' or 'at <HH:MM> <message>'")
}

// ParseAbsoluteTime parses a time like "14:00" or "14:30:00" and returns duration until that time
func ParseAbsoluteTime(timeStr string) (time.Duration, error) {
	now := time.Now()

	// Try parsing HH:MM:SS
	parts := strings.Split(timeStr, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("invalid time format: %s (use HH:MM or HH:MM:SS)", timeStr)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("invalid hour: %s", parts[0])
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("invalid minute: %s", parts[1])
	}

	second := 0
	if len(parts) == 3 {
		second, err = strconv.Atoi(parts[2])
		if err != nil || second < 0 || second > 59 {
			return 0, fmt.Errorf("invalid second: %s", parts[2])
		}
	}

	// Create target time today
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, now.Location())

	// If target is in the past, schedule for tomorrow
	if target.Before(now) || target.Equal(now) {
		target = target.Add(24 * time.Hour)
	}

	return target.Sub(now), nil
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
