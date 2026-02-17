package command

import (
	"strings"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/internal/scheduler"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

func TestIsCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"/new", true},
		{"/history", true},
		{"/help", true},
		{"/model gpt-4", true},
		{"hello", false},
		{"/ command", false},
		{"", false},
		{"//not a command", false},
	}

	for _, tt := range tests {
		got := IsCommand(tt.text)
		if got != tt.want {
			t.Errorf("IsCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		text    string
		wantCmd string
		wantArg string
	}{
		{"/new", "new", ""},
		{"/history", "history", ""},
		{"/history 10", "history", "10"},
		{"/model gpt-4o", "model", "gpt-4o"},
		{"/remind in 10m do something", "remind", "in 10m do something"},
		{"/help   extra   spaces", "help", "extra   spaces"},
	}

	for _, tt := range tests {
		cmd, arg := ParseCommand(tt.text)
		if cmd != tt.wantCmd {
			t.Errorf("ParseCommand(%q) cmd = %q, want %q", tt.text, cmd, tt.wantCmd)
		}
		if arg != tt.wantArg {
			t.Errorf("ParseCommand(%q) arg = %q, want %q", tt.text, arg, tt.wantArg)
		}
	}
}

func TestHandlerNew(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	// Create a session with messages
	sess := store.GetOrCreate("telegram", "user123")
	sess.AddMessage(types.Message{Text: "Hello"})
	sess.AddMessage(types.Message{Text: "Hi there!", IsBot: true})

	if sess.Len() != 2 {
		t.Fatalf("Expected 2 messages before /new, got %d", sess.Len())
	}

	// Execute /new
	msg := &types.Message{
		Text:    "/new",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": int64(123),
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil || result.Text == "" {
		t.Error("Expected response message")
	}

	// Verify session was cleared
	sess = store.GetOrCreate("telegram", "123")
	if sess.Len() != 0 {
		t.Errorf("Expected 0 messages after /new, got %d", sess.Len())
	}
}

func TestHandlerHistory(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	sess := store.GetOrCreate("telegram", "user123")
	
	// Add some messages
	sess.AddMessage(types.Message{
		Text:      "Hello",
		From:      "user",
		Timestamp: time.Now().Add(-2 * time.Minute),
		IsBot:     false,
	})
	sess.AddMessage(types.Message{
		Text:      "Hi there!",
		Timestamp: time.Now().Add(-1 * time.Minute),
		IsBot:     true,
	})

	msg := &types.Message{
		Text:    "/history",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should contain both messages
	if result == nil {
		t.Fatal("Expected response message")
	}

	if result.Text == "" {
		t.Error("Expected non-empty history response")
	}

	// Should mention the messages
	if !containsString(result.Text, "Hello") {
		t.Error("History should contain 'Hello'")
	}
	if !containsString(result.Text, "Hi there!") {
		t.Error("History should contain 'Hi there!'")
	}
}

func TestHandlerHistoryEmpty(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	msg := &types.Message{
		Text:    "/history",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "newuser",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil || result.Text == "" {
		t.Error("Expected response for empty history")
	}
}

func TestHandlerHelp(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	msg := &types.Message{
		Text:    "/help",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected help response")
	}

	// Should list available commands
	if !containsString(result.Text, "/new") {
		t.Error("Help should mention /new")
	}
	if !containsString(result.Text, "/history") {
		t.Error("Help should mention /history")
	}
}

func TestHandlerUnknownCommand(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	msg := &types.Message{
		Text:    "/unknowncmd",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected response for unknown command")
	}
}

func TestHandlerRemind(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	msg := &types.Message{
		Text:    "/remind in 10m test reminder",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected response")
	}

	// Should confirm the reminder was set
	if !strings.Contains(result.Text, "Reminder set") {
		t.Errorf("Expected confirmation, got: %s", result.Text)
	}

	// Should be in the list
	reminders := sched.List("telegram", "user123")
	if len(reminders) != 1 {
		t.Errorf("Expected 1 reminder, got %d", len(reminders))
	}
}

func TestHandlerRemindInvalid(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	msg := &types.Message{
		Text:    "/remind invalid format",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should show error and usage
	if !strings.Contains(result.Text, "Usage") {
		t.Errorf("Expected usage help, got: %s", result.Text)
	}
}

func TestHandlerReminders(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	// Add a reminder directly
	sched.AddReminder("telegram", "user123", 1*time.Hour, "Test reminder")

	msg := &types.Message{
		Text:    "/reminders",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if !strings.Contains(result.Text, "Test reminder") {
		t.Errorf("Expected reminder in list, got: %s", result.Text)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
