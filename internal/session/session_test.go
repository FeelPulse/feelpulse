package session

import (
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("NewStore() returned nil")
	}
}

func TestSessionKey(t *testing.T) {
	tests := []struct {
		channel string
		userID  string
		want    string
	}{
		{"telegram", "123", "telegram:123"},
		{"discord", "456", "discord:456"},
		{"slack", "user@test", "slack:user@test"},
	}

	for _, tt := range tests {
		got := SessionKey(tt.channel, tt.userID)
		if got != tt.want {
			t.Errorf("SessionKey(%q, %q) = %q, want %q", tt.channel, tt.userID, got, tt.want)
		}
	}
}

func TestStoreGetOrCreate(t *testing.T) {
	store := NewStore()

	// First call should create new session
	sess1 := store.GetOrCreate("telegram", "user123")
	if sess1 == nil {
		t.Fatal("GetOrCreate returned nil")
	}
	if len(sess1.Messages) != 0 {
		t.Errorf("New session should have 0 messages, got %d", len(sess1.Messages))
	}

	// Second call should return same session
	sess2 := store.GetOrCreate("telegram", "user123")
	if sess1 != sess2 {
		t.Error("GetOrCreate should return same session for same key")
	}

	// Different user should get different session
	sess3 := store.GetOrCreate("telegram", "user456")
	if sess1 == sess3 {
		t.Error("Different users should get different sessions")
	}
}

func TestSessionAddMessage(t *testing.T) {
	sess := &Session{
		Key:       "telegram:user123",
		CreatedAt: time.Now(),
	}

	msg1 := types.Message{
		ID:      "1",
		Text:    "Hello",
		From:    "user123",
		Channel: "telegram",
		IsBot:   false,
	}
	sess.AddMessage(msg1)

	if len(sess.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(sess.Messages))
	}

	msg2 := types.Message{
		ID:      "2",
		Text:    "Hi there!",
		Channel: "telegram",
		IsBot:   true,
	}
	sess.AddMessage(msg2)

	if len(sess.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(sess.Messages))
	}

	// Verify order
	if sess.Messages[0].Text != "Hello" {
		t.Errorf("First message text = %q, want 'Hello'", sess.Messages[0].Text)
	}
	if sess.Messages[1].Text != "Hi there!" {
		t.Errorf("Second message text = %q, want 'Hi there!'", sess.Messages[1].Text)
	}
}

func TestSessionGetHistory(t *testing.T) {
	sess := &Session{
		Key:       "telegram:user123",
		CreatedAt: time.Now(),
	}

	// Add multiple messages
	for i := 0; i < 5; i++ {
		sess.AddMessage(types.Message{
			ID:    string(rune('0' + i)),
			Text:  "Message " + string(rune('0'+i)),
			IsBot: i%2 == 1,
		})
	}

	// Get all history
	history := sess.GetHistory(10)
	if len(history) != 5 {
		t.Errorf("GetHistory(10) returned %d messages, want 5", len(history))
	}

	// Get limited history
	history = sess.GetHistory(3)
	if len(history) != 3 {
		t.Errorf("GetHistory(3) returned %d messages, want 3", len(history))
	}

	// Should return most recent messages
	if history[0].Text != "Message 2" {
		t.Errorf("First message in limited history = %q, want 'Message 2'", history[0].Text)
	}
}

func TestSessionClear(t *testing.T) {
	sess := &Session{
		Key:       "telegram:user123",
		CreatedAt: time.Now(),
	}

	sess.AddMessage(types.Message{Text: "Hello"})
	sess.AddMessage(types.Message{Text: "World"})

	if len(sess.Messages) != 2 {
		t.Fatalf("Expected 2 messages before clear, got %d", len(sess.Messages))
	}

	sess.Clear()

	if len(sess.Messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(sess.Messages))
	}
}

func TestStoreDelete(t *testing.T) {
	store := NewStore()

	sess := store.GetOrCreate("telegram", "user123")
	sess.AddMessage(types.Message{Text: "Hello"})

	store.Delete("telegram", "user123")

	// Getting the session again should create a new empty one
	sess2 := store.GetOrCreate("telegram", "user123")
	if len(sess2.Messages) != 0 {
		t.Errorf("New session after delete should have 0 messages, got %d", len(sess2.Messages))
	}
}

func TestMaxHistoryLimit(t *testing.T) {
	sess := &Session{
		Key:       "telegram:user123",
		CreatedAt: time.Now(),
		MaxHistory: 3,
	}

	// Add more messages than max
	for i := 0; i < 5; i++ {
		sess.AddMessage(types.Message{
			Text: "Message " + string(rune('A'+i)),
		})
	}

	// Should only keep last MaxHistory messages
	if len(sess.Messages) != 3 {
		t.Errorf("Expected %d messages after limit, got %d", 3, len(sess.Messages))
	}

	// Should be the most recent messages
	if sess.Messages[0].Text != "Message C" {
		t.Errorf("First message = %q, want 'Message C'", sess.Messages[0].Text)
	}
	if sess.Messages[2].Text != "Message E" {
		t.Errorf("Last message = %q, want 'Message E'", sess.Messages[2].Text)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewStore()
	done := make(chan bool, 10)

	// Concurrent access should be safe
	for i := 0; i < 10; i++ {
		go func(id int) {
			sess := store.GetOrCreate("telegram", "user")
			sess.AddMessage(types.Message{Text: "msg"})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	sess := store.GetOrCreate("telegram", "user")
	if len(sess.Messages) != 10 {
		t.Errorf("Expected 10 messages after concurrent adds, got %d", len(sess.Messages))
	}
}
