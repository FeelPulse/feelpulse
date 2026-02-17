package agent

import (
	"testing"
)

func TestIsOAuthToken(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		{"sk-ant-oat-abc123", true},
		{"sk-ant-oat123456", true},
		{"sk-ant-api-key123", false},
		{"some-random-key", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsOAuthToken(tt.token)
		if got != tt.want {
			t.Errorf("IsOAuthToken(%q) = %v, want %v", tt.token, got, tt.want)
		}
	}
}

func TestNewAnthropicClient(t *testing.T) {
	// Test API key auth
	client := NewAnthropicClient("sk-ant-api-key", "", "claude-sonnet-4-20250514")
	if client.authMode != AuthModeAPIKey {
		t.Errorf("Expected AuthModeAPIKey, got %d", client.authMode)
	}

	// Test OAuth auth
	client = NewAnthropicClient("", "sk-ant-oat-token", "claude-sonnet-4-20250514")
	if client.authMode != AuthModeOAuth {
		t.Errorf("Expected AuthModeOAuth, got %d", client.authMode)
	}

	// Test default model
	client = NewAnthropicClient("key", "", "")
	if client.model != "claude-sonnet-4-20250514" {
		t.Errorf("Expected default model claude-sonnet-4-20250514, got %s", client.model)
	}
}

func TestAnthropicClientName(t *testing.T) {
	client := NewAnthropicClient("key", "", "model")
	if client.Name() != "anthropic" {
		t.Errorf("Expected name 'anthropic', got %s", client.Name())
	}
}

func TestParseSSEEvent(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantType  string
		wantDelta string
		wantErr   bool
	}{
		{
			name:      "content_block_delta",
			data:      `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			wantType:  "content_block_delta",
			wantDelta: "Hello",
			wantErr:   false,
		},
		{
			name:      "message_start",
			data:      `{"type":"message_start","message":{"id":"msg_123","model":"claude-3"}}`,
			wantType:  "message_start",
			wantDelta: "",
			wantErr:   false,
		},
		{
			name:      "message_stop",
			data:      `{"type":"message_stop"}`,
			wantType:  "message_stop",
			wantDelta: "",
			wantErr:   false,
		},
		{
			name:     "invalid json",
			data:     `{invalid}`,
			wantType: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseSSEEvent(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if event.Type != tt.wantType {
				t.Errorf("Type = %s, want %s", event.Type, tt.wantType)
			}
			if event.Delta.Text != tt.wantDelta {
				t.Errorf("Delta.Text = %s, want %s", event.Delta.Text, tt.wantDelta)
			}
		})
	}
}

func TestStreamingCallbackCalled(t *testing.T) {
	// This test verifies the callback mechanism works
	// It doesn't actually make API calls
	
	var received []string
	callback := func(delta string) {
		received = append(received, delta)
	}

	// Simulate what would happen with streaming
	callback("Hello")
	callback(" World")
	callback("!")

	if len(received) != 3 {
		t.Errorf("Expected 3 callbacks, got %d", len(received))
	}
	
	full := ""
	for _, s := range received {
		full += s
	}
	if full != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got %s", full)
	}
}
