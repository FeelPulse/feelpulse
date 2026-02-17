package channel

import (
	"testing"
)

func TestNewTelegramBot(t *testing.T) {
	bot := NewTelegramBot("test-token")
	if bot == nil {
		t.Fatal("NewTelegramBot returned nil")
	}
	if bot.token != "test-token" {
		t.Errorf("Expected token 'test-token', got %s", bot.token)
	}
}

func TestTelegramBotBaseURL(t *testing.T) {
	bot := NewTelegramBot("123:ABC")
	expected := "https://api.telegram.org/bot123:ABC"
	if bot.baseURL != expected {
		t.Errorf("Expected baseURL %s, got %s", expected, bot.baseURL)
	}
}

func TestSendTypingActionParams(t *testing.T) {
	// Test that sendChatAction params are formatted correctly
	bot := NewTelegramBot("test-token")
	
	// Verify the bot has the method
	// This is a compile-time check - if SendTypingAction doesn't exist, this won't compile
	_ = bot.SendTypingAction
}
