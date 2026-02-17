package agent

import (
	"testing"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

func TestNewOpenAIClient(t *testing.T) {
	client := NewOpenAIClient("sk-test-key", "gpt-4o")
	if client == nil {
		t.Fatal("NewOpenAIClient returned nil")
	}
	if client.model != "gpt-4o" {
		t.Errorf("Expected model gpt-4o, got %s", client.model)
	}
}

func TestOpenAIClientDefaultModel(t *testing.T) {
	client := NewOpenAIClient("sk-test-key", "")
	if client.model != "gpt-4o" {
		t.Errorf("Expected default model gpt-4o, got %s", client.model)
	}
}

func TestOpenAIClientName(t *testing.T) {
	client := NewOpenAIClient("sk-test-key", "gpt-4")
	if client.Name() != "openai" {
		t.Errorf("Expected name 'openai', got %s", client.Name())
	}
}

func TestConvertMessagesToOpenAI(t *testing.T) {
	messages := []types.Message{
		{Text: "Hello", IsBot: false},
		{Text: "Hi there!", IsBot: true},
		{Text: "How are you?", IsBot: false},
	}

	client := NewOpenAIClient("key", "gpt-4o")
	openaiMsgs := client.convertMessages(messages)

	if len(openaiMsgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(openaiMsgs))
	}

	expected := []struct {
		role    string
		content string
	}{
		{"user", "Hello"},
		{"assistant", "Hi there!"},
		{"user", "How are you?"},
	}

	for i, exp := range expected {
		if openaiMsgs[i].Role != exp.role {
			t.Errorf("Message %d: role = %s, want %s", i, openaiMsgs[i].Role, exp.role)
		}
		if openaiMsgs[i].Content != exp.content {
			t.Errorf("Message %d: content = %s, want %s", i, openaiMsgs[i].Content, exp.content)
		}
	}
}

func TestOpenAIParseSSEData(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantDelta string
		wantDone  bool
		wantErr   bool
	}{
		{
			name:      "content delta",
			data:      `{"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"}}]}`,
			wantDelta: "Hello",
			wantDone:  false,
		},
		{
			name:      "done marker",
			data:      "[DONE]",
			wantDelta: "",
			wantDone:  true,
		},
		{
			name:      "empty choices",
			data:      `{"id":"chatcmpl-123","choices":[]}`,
			wantDelta: "",
			wantDone:  false,
		},
		{
			name:    "invalid json",
			data:    "{invalid}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta, done, err := parseOpenAISSE(tt.data)
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
			if delta != tt.wantDelta {
				t.Errorf("delta = %q, want %q", delta, tt.wantDelta)
			}
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
		})
	}
}
