package channel

import (
	"testing"
)

func TestNewTelegramBot(t *testing.T) {
	bot := NewTelegramBot("test-token", nil)
	if bot == nil {
		t.Fatal("NewTelegramBot returned nil")
	}
	if bot.token != "test-token" {
		t.Errorf("Expected token 'test-token', got %s", bot.token)
	}
}

func TestTelegramBotBaseURL(t *testing.T) {
	bot := NewTelegramBot("123:ABC", nil)
	expected := "https://api.telegram.org/bot123:ABC"
	if bot.baseURL != expected {
		t.Errorf("Expected baseURL %s, got %s", expected, bot.baseURL)
	}
}

func TestSendTypingActionParams(t *testing.T) {
	// Test that sendChatAction params are formatted correctly
	bot := NewTelegramBot("test-token", nil)
	
	// Verify the bot has the method
	// This is a compile-time check - if SendTypingAction doesn't exist, this won't compile
	_ = bot.SendTypingAction
}

func TestIsUserAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedUsers []string
		username     string
		expected     bool
	}{
		{
			name:         "empty allowlist allows all",
			allowedUsers: []string{},
			username:     "anyuser",
			expected:     true,
		},
		{
			name:         "nil allowlist allows all",
			allowedUsers: nil,
			username:     "anyuser",
			expected:     true,
		},
		{
			name:         "user in allowlist is allowed",
			allowedUsers: []string{"alice", "bob"},
			username:     "alice",
			expected:     true,
		},
		{
			name:         "user not in allowlist is blocked",
			allowedUsers: []string{"alice", "bob"},
			username:     "charlie",
			expected:     false,
		},
		{
			name:         "case sensitive matching",
			allowedUsers: []string{"Alice"},
			username:     "alice",
			expected:     false,
		},
		{
			name:         "empty username blocked if allowlist set",
			allowedUsers: []string{"alice"},
			username:     "",
			expected:     false,
		},
		{
			name:         "empty username allowed if allowlist empty",
			allowedUsers: []string{},
			username:     "",
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := NewTelegramBot("test-token", nil)
			bot.SetAllowedUsers(tt.allowedUsers)

			result := bot.IsUserAllowed(tt.username)
			if result != tt.expected {
				t.Errorf("IsUserAllowed(%q) with allowlist %v = %v, want %v",
					tt.username, tt.allowedUsers, result, tt.expected)
			}
		})
	}
}

func TestSetAllowedUsers(t *testing.T) {
	bot := NewTelegramBot("test-token", nil)
	
	// Initially no allowlist
	if !bot.IsUserAllowed("anyone") {
		t.Error("expected anyone to be allowed with no allowlist")
	}

	// Set allowlist
	bot.SetAllowedUsers([]string{"admin", "user1"})
	
	if !bot.IsUserAllowed("admin") {
		t.Error("admin should be allowed")
	}
	if !bot.IsUserAllowed("user1") {
		t.Error("user1 should be allowed")
	}
	if bot.IsUserAllowed("stranger") {
		t.Error("stranger should not be allowed")
	}
}
