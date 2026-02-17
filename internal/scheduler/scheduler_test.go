package scheduler

import (
	"sync"
	"testing"
	"time"
)

func TestNewScheduler(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"10m", 10 * time.Minute, false},
		{"1h", 1 * time.Hour, false},
		{"30s", 30 * time.Second, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"1d", 24 * time.Hour, false},
		{"2d", 48 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseDuration(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("ParseDuration(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseRemindCommand(t *testing.T) {
	tests := []struct {
		input       string
		wantIn      string
		wantMessage string
		err         bool
	}{
		{"in 10m do something", "10m", "do something", false},
		{"in 1h check email", "1h", "check email", false},
		{"in 30s ping", "30s", "ping", false},
		{"in 2d review code", "2d", "review code", false},
		{"invalid format", "", "", true},
		{"in 10m", "", "", true}, // no message
	}

	for _, tt := range tests {
		gotIn, gotMsg, err := ParseRemindCommand(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("ParseRemindCommand(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRemindCommand(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if gotIn != tt.wantIn {
			t.Errorf("ParseRemindCommand(%q) in = %q, want %q", tt.input, gotIn, tt.wantIn)
		}
		if gotMsg != tt.wantMessage {
			t.Errorf("ParseRemindCommand(%q) message = %q, want %q", tt.input, gotMsg, tt.wantMessage)
		}
	}
}

func TestSchedulerAddReminder(t *testing.T) {
	s := New()
	defer s.Stop()

	id, err := s.AddReminder("telegram", "user123", 1*time.Hour, "Test reminder")
	if err != nil {
		t.Fatalf("AddReminder error: %v", err)
	}

	if id == "" {
		t.Error("Expected non-empty ID")
	}

	// Should be in the list
	reminders := s.List("telegram", "user123")
	if len(reminders) != 1 {
		t.Errorf("Expected 1 reminder, got %d", len(reminders))
	}
}

func TestSchedulerCancelReminder(t *testing.T) {
	s := New()
	defer s.Stop()

	id, _ := s.AddReminder("telegram", "user123", 1*time.Hour, "Test")

	ok := s.Cancel(id)
	if !ok {
		t.Error("Cancel should return true for existing reminder")
	}

	// Should not be in the list
	reminders := s.List("telegram", "user123")
	if len(reminders) != 0 {
		t.Errorf("Expected 0 reminders after cancel, got %d", len(reminders))
	}

	// Cancel again should return false
	ok = s.Cancel(id)
	if ok {
		t.Error("Cancel should return false for already-cancelled reminder")
	}
}

func TestSchedulerReminderFires(t *testing.T) {
	// Use fast tick interval for testing
	s := NewWithInterval(10 * time.Millisecond)
	defer s.Stop()

	var mu sync.Mutex
	var fired bool
	var firedMsg string

	s.SetHandler(func(r *Reminder) {
		mu.Lock()
		fired = true
		firedMsg = r.Message
		mu.Unlock()
	})

	s.Start()

	_, err := s.AddReminder("telegram", "user123", 50*time.Millisecond, "Fire test")
	if err != nil {
		t.Fatalf("AddReminder error: %v", err)
	}

	// Wait for it to fire
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	wasFired := fired
	msg := firedMsg
	mu.Unlock()

	if !wasFired {
		t.Error("Reminder did not fire")
	}
	if msg != "Fire test" {
		t.Errorf("Expected message 'Fire test', got %q", msg)
	}
}

func TestSchedulerListReminders(t *testing.T) {
	s := New()
	defer s.Stop()

	s.AddReminder("telegram", "user1", 1*time.Hour, "R1")
	s.AddReminder("telegram", "user1", 2*time.Hour, "R2")
	s.AddReminder("telegram", "user2", 1*time.Hour, "R3")
	s.AddReminder("discord", "user1", 1*time.Hour, "R4")

	// User1 on telegram should have 2
	list := s.List("telegram", "user1")
	if len(list) != 2 {
		t.Errorf("Expected 2 reminders for user1@telegram, got %d", len(list))
	}

	// User2 on telegram should have 1
	list = s.List("telegram", "user2")
	if len(list) != 1 {
		t.Errorf("Expected 1 reminder for user2@telegram, got %d", len(list))
	}
}

func TestReminderString(t *testing.T) {
	r := &Reminder{
		ID:       "abc123",
		Message:  "Test message",
		FireAt:   time.Now().Add(30 * time.Minute),
	}

	str := r.String()
	if str == "" {
		t.Error("Reminder.String() should not be empty")
	}
}
