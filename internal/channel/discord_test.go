package channel

import (
	"testing"
)

func TestNewDiscordBot(t *testing.T) {
	bot := NewDiscordBot("test-token")
	if bot == nil {
		t.Fatal("NewDiscordBot returned nil")
	}
	if bot.token != "test-token" {
		t.Errorf("Expected token 'test-token', got %s", bot.token)
	}
}

func TestDiscordBotSetHandler(t *testing.T) {
	bot := NewDiscordBot("test-token")
	
	bot.SetHandler(func(msg *Message) (*Message, error) {
		return nil, nil
	})

	if bot.handler == nil {
		t.Error("Handler should be set")
	}
}

func TestDiscordMessageToInternal(t *testing.T) {
	// Test conversion of Discord message to internal Message type
	dm := &DiscordMessage{
		ID:        "123456",
		Content:   "Hello, bot!",
		ChannelID: "chan789",
		Author: &DiscordUser{
			ID:       "user456",
			Username: "testuser",
		},
	}

	msg := discordToMessage(dm)

	if msg.ID != "123456" {
		t.Errorf("ID = %s, want 123456", msg.ID)
	}
	if msg.Text != "Hello, bot!" {
		t.Errorf("Text = %s, want 'Hello, bot!'", msg.Text)
	}
	if msg.Channel != "discord" {
		t.Errorf("Channel = %s, want discord", msg.Channel)
	}
	if msg.From != "testuser" {
		t.Errorf("From = %s, want testuser", msg.From)
	}
	if msg.Metadata["channel_id"] != "chan789" {
		t.Errorf("channel_id = %v, want chan789", msg.Metadata["channel_id"])
	}
	if msg.Metadata["user_id"] != "user456" {
		t.Errorf("user_id = %v, want user456", msg.Metadata["user_id"])
	}
}
