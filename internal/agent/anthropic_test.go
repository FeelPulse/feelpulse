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

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"hi", 2, "hi"},
		{"hello", 5, "hello"},
	}

	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestTruncateJSON(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{`{"key":"value"}`, 100, `{"key":"value"}`},
		{`{"key":"value"}`, 5, `{"key...`},
	}

	for _, tt := range tests {
		got := truncateJSON([]byte(tt.input), tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateJSON(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestToolExecutorType(t *testing.T) {
	// Test that ToolExecutor function signature works correctly
	var executor ToolExecutor = func(name string, input map[string]any) (string, error) {
		if name == "test_tool" {
			query, _ := input["query"].(string)
			return "Result for: " + query, nil
		}
		return "", nil
	}

	result, err := executor("test_tool", map[string]any{"query": "hello"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "Result for: hello" {
		t.Errorf("Expected 'Result for: hello', got %s", result)
	}
}

func TestContentBlockTypes(t *testing.T) {
	// Test ContentBlock struct
	textBlock := ContentBlock{
		Type: "text",
		Text: "Hello world",
	}
	if textBlock.Type != "text" {
		t.Errorf("Expected type 'text', got %s", textBlock.Type)
	}

	toolUseBlock := ContentBlock{
		Type:  "tool_use",
		ID:    "tool_123",
		Name:  "web_search",
		Input: []byte(`{"query":"test"}`),
	}
	if toolUseBlock.Type != "tool_use" {
		t.Errorf("Expected type 'tool_use', got %s", toolUseBlock.Type)
	}

	toolResultBlock := ContentBlock{
		Type:      "tool_result",
		ToolUseID: "tool_123",
		Content:   "Search results here",
	}
	if toolResultBlock.Type != "tool_result" {
		t.Errorf("Expected type 'tool_result', got %s", toolResultBlock.Type)
	}
}

func TestAnthropicToolDefinition(t *testing.T) {
	tool := AnthropicTool{
		Name:        "web_search",
		Description: "Search the web",
		InputSchema: []byte(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
	}

	if tool.Name != "web_search" {
		t.Errorf("Expected name 'web_search', got %s", tool.Name)
	}
	if tool.Description != "Search the web" {
		t.Errorf("Expected description 'Search the web', got %s", tool.Description)
	}
	if len(tool.InputSchema) == 0 {
		t.Error("Expected non-empty InputSchema")
	}
}
