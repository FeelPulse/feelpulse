package command

import (
	"testing"
	"time"

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
