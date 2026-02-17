package gateway

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOpenAIRequest_Parse(t *testing.T) {
	jsonData := `{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "Hello!"}
		],
		"max_tokens": 100,
		"temperature": 0.7,
		"stream": false
	}`

	var req OpenAIRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if req.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", req.Model)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("Expected first message role 'system', got '%s'", req.Messages[0].Role)
	}

	if req.MaxTokens != 100 {
		t.Errorf("Expected max_tokens 100, got %d", req.MaxTokens)
	}

	if req.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", req.Temperature)
	}
}

func TestOpenAIResponse_Marshal(t *testing.T) {
	resp := OpenAIResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: "Hello! How can I help you today?",
				},
				FinishReason: "stop",
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 8,
			TotalTokens:      18,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	// Verify it can be parsed back
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse marshaled response: %v", err)
	}

	if parsed["id"] != "chatcmpl-123" {
		t.Errorf("Expected id 'chatcmpl-123', got '%v'", parsed["id"])
	}

	if parsed["object"] != "chat.completion" {
		t.Errorf("Expected object 'chat.completion', got '%v'", parsed["object"])
	}
}

func TestOpenAIError_Marshal(t *testing.T) {
	errResp := OpenAIErrorResponse{
		Error: OpenAIError{
			Message: "Invalid API key",
			Type:    "invalid_request_error",
			Code:    "invalid_api_key",
		},
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("Failed to marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse marshaled error: %v", err)
	}

	errObj, ok := parsed["error"].(map[string]any)
	if !ok {
		t.Fatal("Expected error object in response")
	}

	if errObj["message"] != "Invalid API key" {
		t.Errorf("Unexpected error message: %v", errObj["message"])
	}
}

func TestConvertOpenAIToInternal(t *testing.T) {
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		},
	}

	messages, systemPrompt := convertOpenAIToInternal(req)

	if systemPrompt != "Be helpful" {
		t.Errorf("Expected system prompt 'Be helpful', got '%s'", systemPrompt)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages (excluding system), got %d", len(messages))
	}

	if messages[0].IsBot {
		t.Error("First user message should not be marked as bot")
	}

	if !messages[1].IsBot {
		t.Error("Assistant message should be marked as bot")
	}

	if messages[2].Text != "How are you?" {
		t.Errorf("Unexpected last message: %s", messages[2].Text)
	}
}

func TestConvertOpenAIToInternal_NoSystem(t *testing.T) {
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	messages, systemPrompt := convertOpenAIToInternal(req)

	if systemPrompt != "" {
		t.Errorf("Expected empty system prompt, got '%s'", systemPrompt)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
}

func TestGenerateCompletionID(t *testing.T) {
	id1 := generateCompletionID()
	id2 := generateCompletionID()

	if id1 == id2 {
		t.Error("Expected unique IDs")
	}

	if len(id1) < 20 {
		t.Errorf("ID too short: %s", id1)
	}

	// Check prefix
	if id1[:9] != "chatcmpl-" {
		t.Errorf("Expected 'chatcmpl-' prefix, got: %s", id1)
	}
}

func TestMapAnthropicModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gpt-4", "claude-sonnet-4-20250514"},
		{"gpt-4-turbo", "claude-sonnet-4-20250514"},
		{"gpt-3.5-turbo", "claude-3-haiku-20240307"},
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"},
		{"unknown-model", "claude-sonnet-4-20250514"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapToAnthropicModel(tt.input)
			if result != tt.expected {
				t.Errorf("mapToAnthropicModel(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
