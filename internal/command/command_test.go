package command

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/scheduler"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/usage"
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

func TestHandlerUsage(t *testing.T) {
	store := session.NewStore()
	tracker := usage.NewTracker()

	handler := NewHandler(store, nil)
	handler.SetUsageTracker(tracker)

	// Record some usage
	tracker.Record("telegram", "user123", 100, 50, "claude-sonnet-4")
	tracker.Record("telegram", "user123", 200, 100, "gpt-4o")

	msg := &types.Message{
		Text:    "/usage",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should contain usage info
	if !strings.Contains(result.Text, "450") { // total tokens
		t.Errorf("Expected total tokens in output, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "2") { // request count
		t.Errorf("Expected request count in output, got: %s", result.Text)
	}
}

func TestHandlerExport(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	// Add some messages
	sess := store.GetOrCreate("telegram", "user123")
	sess.AddMessage(types.Message{
		Text:      "Hello!",
		From:      "user",
		Timestamp: time.Now().Add(-5 * time.Minute),
		IsBot:     false,
	})
	sess.AddMessage(types.Message{
		Text:      "Hi there! How can I help?",
		Timestamp: time.Now().Add(-4 * time.Minute),
		IsBot:     true,
	})

	msg := &types.Message{
		Text:    "/export",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
			"chat_id": int64(12345),
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected response")
	}

	// Check export metadata
	if result.Metadata == nil {
		t.Fatal("Expected metadata in response")
	}

	export, ok := result.Metadata["export"].(bool)
	if !ok || !export {
		t.Error("Expected export=true in metadata")
	}

	filename, ok := result.Metadata["filename"].(string)
	if !ok || filename == "" {
		t.Error("Expected filename in metadata")
	}

	// Check content
	if !strings.Contains(result.Text, "Hello!") {
		t.Error("Export should contain user message")
	}
	if !strings.Contains(result.Text, "Hi there!") {
		t.Error("Export should contain bot message")
	}
	if !strings.Contains(result.Text, "User:") {
		t.Error("Export should contain User: label")
	}
	if !strings.Contains(result.Text, "AI:") {
		t.Error("Export should contain AI: label")
	}
}

func TestHandlerExportEmpty(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	msg := &types.Message{
		Text:    "/export",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "newuser",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should indicate no conversation
	if result.Metadata != nil {
		if _, ok := result.Metadata["export"]; ok {
			t.Error("Should not have export metadata for empty conversation")
		}
	}
}

func TestFormatExport(t *testing.T) {
	messages := []types.Message{
		{
			Text:      "Hello",
			Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			IsBot:     false,
		},
		{
			Text:      "Hi there!",
			Timestamp: time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC),
			IsBot:     true,
		},
	}

	export := FormatExport(messages)

	if !strings.Contains(export, "[2024-01-15 10:30:00] User: Hello") {
		t.Errorf("Export format incorrect. Got: %s", export)
	}
	if !strings.Contains(export, "[2024-01-15 10:31:00] AI: Hi there!") {
		t.Errorf("Export format incorrect. Got: %s", export)
	}
	if !strings.Contains(export, "# Messages: 2") {
		t.Error("Export should include message count")
	}
}

func TestHandlerTTS(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	tests := []struct {
		name     string
		args     string
		wantTTS  *bool
		wantText string
	}{
		{"enable", "on", boolPtr(true), "TTS Enabled"},
		{"disable", "off", boolPtr(false), "TTS Disabled"},
		{"status on", "", nil, "TTS Status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &types.Message{
				Text:    "/tts " + tt.args,
				Channel: "telegram",
				Metadata: map[string]any{
					"user_id": "user123",
				},
			}

			result, err := handler.Handle(msg)
			if err != nil {
				t.Fatalf("Handle error: %v", err)
			}

			if !strings.Contains(result.Text, tt.wantText) {
				t.Errorf("Expected %q in response, got: %s", tt.wantText, result.Text)
			}

			// Check session state
			sess, _ := store.Get("telegram", "user123")
			if sess != nil && tt.wantTTS != nil {
				got := sess.GetTTS()
				if got == nil || *got != *tt.wantTTS {
					t.Errorf("TTS state = %v, want %v", got, *tt.wantTTS)
				}
			}
		})
	}
}

func TestHandlerProfile(t *testing.T) {
	store := session.NewStore()
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Profiles: map[string]string{
				"friendly": "/path/to/friendly.md",
				"formal":   "/path/to/formal.md",
			},
		},
	}
	handler := NewHandler(store, cfg)

	t.Run("list", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/profile list",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "friendly") {
			t.Error("List should contain 'friendly' profile")
		}
		if !strings.Contains(result.Text, "formal") {
			t.Error("List should contain 'formal' profile")
		}
	})

	t.Run("use", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/profile use friendly",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Switched") {
			t.Errorf("Expected switch confirmation, got: %s", result.Text)
		}

		sess, _ := store.Get("telegram", "user123")
		if sess.GetProfile() != "friendly" {
			t.Errorf("Profile = %q, want 'friendly'", sess.GetProfile())
		}
	})

	t.Run("use unknown", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/profile use unknown",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user456",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Unknown profile") {
			t.Errorf("Expected error for unknown profile, got: %s", result.Text)
		}
	})

	t.Run("reset", func(t *testing.T) {
		// First set a profile
		sess := store.GetOrCreate("telegram", "user789")
		sess.SetProfile("friendly")

		msg := &types.Message{
			Text:    "/profile reset",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user789",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Reset") {
			t.Errorf("Expected reset confirmation, got: %s", result.Text)
		}

		if sess.GetProfile() != "" {
			t.Errorf("Profile = %q, want empty", sess.GetProfile())
		}
	})
}

func TestHandlerCancel(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	// Add a reminder
	id, _ := sched.AddReminder("telegram", "user123", 1*time.Hour, "Test reminder")
	shortID := id[:8]

	t.Run("cancel by short id", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/cancel " + shortID,
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Cancelled") {
			t.Errorf("Expected cancellation, got: %s", result.Text)
		}

		reminders := sched.List("telegram", "user123")
		if len(reminders) != 0 {
			t.Error("Reminder should be cancelled")
		}
	})

	t.Run("cancel unknown", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/cancel unknown",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "not found") {
			t.Errorf("Expected not found error, got: %s", result.Text)
		}
	})

	t.Run("cancel no args", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/cancel",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Usage") {
			t.Errorf("Expected usage help, got: %s", result.Text)
		}
	})
}

func TestHandlerRemindAbsoluteTime(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	msg := &types.Message{
		Text:    "/remind at 14:30 check email",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if !strings.Contains(result.Text, "Reminder set") {
		t.Errorf("Expected confirmation, got: %s", result.Text)
	}

	reminders := sched.List("telegram", "user123")
	if len(reminders) != 1 {
		t.Errorf("Expected 1 reminder, got %d", len(reminders))
	}
}

func TestHandlerBrowse(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	t.Run("no browser", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/browse https://example.com",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "not enabled") {
			t.Errorf("Expected not enabled error, got: %s", result.Text)
		}
	})

	t.Run("no args", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/browse",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Usage") {
			t.Errorf("Expected usage hint, got: %s", result.Text)
		}
	})

	t.Run("with mock browser", func(t *testing.T) {
		mockBrowser := &mockBrowserNavigator{
			result: "title: Example\n\ncontent: Hello World!",
		}
		handler.SetBrowser(mockBrowser)

		msg := &types.Message{
			Text:    "/browse https://example.com",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Hello World!") {
			t.Errorf("Expected page content, got: %s", result.Text)
		}
	})

	t.Run("auto-add https", func(t *testing.T) {
		mockBrowser := &mockBrowserNavigator{
			lastURL: "",
			result:  "title: Test\n\ncontent: Test",
		}
		handler.SetBrowser(mockBrowser)

		msg := &types.Message{
			Text:    "/browse example.com",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		_, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if mockBrowser.lastURL != "https://example.com" {
			t.Errorf("Expected https:// prefix, got: %s", mockBrowser.lastURL)
		}
	})
}

func TestHandlerCompact(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	t.Run("no compactor", func(t *testing.T) {
		msg := &types.Message{
			Text:    "/compact",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "user123",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "not enabled") {
			t.Errorf("Expected not enabled error, got: %s", result.Text)
		}
	})

	t.Run("empty session", func(t *testing.T) {
		handler.SetCompactor(&mockCompactor{})

		msg := &types.Message{
			Text:    "/compact",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "newuser",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "No conversation") {
			t.Errorf("Expected no conversation message, got: %s", result.Text)
		}
	})

	t.Run("compacts messages", func(t *testing.T) {
		// Add messages to a session
		sess := store.GetOrCreate("telegram", "compactuser")
		for i := 0; i < 20; i++ {
			sess.AddMessage(types.Message{
				Text:  fmt.Sprintf("Message %d", i),
				IsBot: i%2 == 1,
			})
		}

		handler.SetCompactor(&mockCompactor{
			compactResult: []types.Message{
				{Text: "Summary of conversation", IsBot: true},
				{Text: "Message 18", IsBot: false},
				{Text: "Message 19", IsBot: true},
			},
		})

		msg := &types.Message{
			Text:    "/compact",
			Channel: "telegram",
			Metadata: map[string]any{
				"user_id": "compactuser",
			},
		}

		result, err := handler.Handle(msg)
		if err != nil {
			t.Fatalf("Handle error: %v", err)
		}

		if !strings.Contains(result.Text, "Compacted") {
			t.Errorf("Expected compaction confirmation, got: %s", result.Text)
		}

		// Verify session was updated
		messages := sess.GetAllMessages()
		if len(messages) != 3 {
			t.Errorf("Expected 3 messages after compaction, got %d", len(messages))
		}
	})
}

func TestHandlerRemindNoArgs(t *testing.T) {
	store := session.NewStore()
	sched := scheduler.New()
	defer sched.Stop()

	handler := NewHandler(store, nil)
	handler.SetScheduler(sched)

	msg := &types.Message{
		Text:    "/remind",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should show usage hint
	if !strings.Contains(result.Text, "Usage") {
		t.Errorf("Expected usage hint for /remind, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "/remind") && !strings.Contains(result.Text, "HH:MM") {
		t.Error("Usage hint should include examples")
	}
}

func TestHandlerModelUnknown(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	msg := &types.Message{
		Text:    "/model totally-unknown-model",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should show error and mention available models
	if !strings.Contains(result.Text, "Unknown model") {
		t.Errorf("Expected 'Unknown model' error, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "/models") {
		t.Error("Should suggest using /models command")
	}
}

func TestHandlerProfileUseNoName(t *testing.T) {
	store := session.NewStore()
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Profiles: map[string]string{
				"friendly": "/path/to/friendly.md",
				"formal":   "/path/to/formal.md",
			},
		},
	}
	handler := NewHandler(store, cfg)

	msg := &types.Message{
		Text:    "/profile use",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should show usage hint
	if !strings.Contains(result.Text, "Usage") {
		t.Errorf("Expected usage hint, got: %s", result.Text)
	}
}

func TestHandlerBrowseWithScreenshot(t *testing.T) {
	store := session.NewStore()
	handler := NewHandler(store, nil)

	mockBrowser := &mockBrowserNavigatorWithScreenshot{
		navigateResult:   "title: Example\n\ncontent: Hello World!",
		screenshotResult: "/tmp/screenshot-123.png",
	}
	handler.SetBrowser(mockBrowser)

	msg := &types.Message{
		Text:    "/browse https://example.com",
		Channel: "telegram",
		Metadata: map[string]any{
			"user_id": "user123",
			"chat_id": int64(12345),
		},
	}

	result, err := handler.Handle(msg)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	// Should contain page content
	if !strings.Contains(result.Text, "Hello World!") {
		t.Errorf("Expected page content, got: %s", result.Text)
	}

	// Should have screenshot metadata for Telegram to send as photo
	if result.Metadata == nil {
		t.Fatal("Expected metadata with screenshot info")
	}

	screenshotPath, ok := result.Metadata["screenshot_path"].(string)
	if !ok || screenshotPath == "" {
		t.Error("Expected screenshot_path in metadata")
	}
}

// Mock implementations for testing
type mockBrowserNavigator struct {
	result  string
	err     error
	lastURL string
}

func (m *mockBrowserNavigator) Navigate(params map[string]interface{}) (string, error) {
	if url, ok := params["url"].(string); ok {
		m.lastURL = url
	}
	return m.result, m.err
}

func (m *mockBrowserNavigator) Screenshot(params map[string]interface{}) (string, error) {
	return "", nil // Basic mock doesn't support screenshot
}

type mockBrowserNavigatorWithScreenshot struct {
	navigateResult   string
	screenshotResult string
	navigateErr      error
	screenshotErr    error
	lastURL          string
}

func (m *mockBrowserNavigatorWithScreenshot) Navigate(params map[string]interface{}) (string, error) {
	if url, ok := params["url"].(string); ok {
		m.lastURL = url
	}
	return m.navigateResult, m.navigateErr
}

func (m *mockBrowserNavigatorWithScreenshot) Screenshot(params map[string]interface{}) (string, error) {
	return m.screenshotResult, m.screenshotErr
}

type mockCompactor struct {
	compactResult []types.Message
	compactErr    error
}

func (m *mockCompactor) CompactIfNeeded(messages []types.Message) ([]types.Message, error) {
	if m.compactResult != nil {
		return m.compactResult, m.compactErr
	}
	return messages, nil
}

func (m *mockCompactor) ForceCompact(messages []types.Message) ([]types.Message, error) {
	if m.compactResult != nil {
		return m.compactResult, m.compactErr
	}
	return messages, nil
}

func boolPtr(b bool) *bool {
	return &b
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
