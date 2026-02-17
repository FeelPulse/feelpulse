package agent

import (
	"errors"
	"testing"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// MockAgent for testing failover
type MockAgent struct {
	name      string
	shouldErr bool
	response  string
}

func (m *MockAgent) Name() string {
	return m.name
}

func (m *MockAgent) Chat(messages []types.Message) (*types.AgentResponse, error) {
	if m.shouldErr {
		return nil, errors.New("mock error")
	}
	return &types.AgentResponse{
		Text:  m.response,
		Model: m.name,
	}, nil
}

func (m *MockAgent) ChatStream(messages []types.Message, systemPrompt string, callback StreamCallback) (*types.AgentResponse, error) {
	return m.Chat(messages)
}

func TestNewFailoverAgent(t *testing.T) {
	primary := &MockAgent{name: "primary", response: "hello"}
	fallback := &MockAgent{name: "fallback", response: "hi there"}

	agent := NewFailoverAgent(primary, fallback)
	if agent == nil {
		t.Fatal("NewFailoverAgent returned nil")
	}
}

func TestFailoverAgentPrimarySuccess(t *testing.T) {
	primary := &MockAgent{name: "primary", response: "hello", shouldErr: false}
	fallback := &MockAgent{name: "fallback", response: "hi there", shouldErr: false}

	agent := NewFailoverAgent(primary, fallback)

	messages := []types.Message{{Text: "test"}}
	resp, err := agent.Chat(messages)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.Model != "primary" {
		t.Errorf("Expected primary model, got %s", resp.Model)
	}
	if resp.Text != "hello" {
		t.Errorf("Expected 'hello', got %s", resp.Text)
	}
}

func TestFailoverAgentFallsBack(t *testing.T) {
	primary := &MockAgent{name: "primary", shouldErr: true}
	fallback := &MockAgent{name: "fallback", response: "hi there", shouldErr: false}

	agent := NewFailoverAgent(primary, fallback)

	messages := []types.Message{{Text: "test"}}
	resp, err := agent.Chat(messages)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.Model != "fallback" {
		t.Errorf("Expected fallback model, got %s", resp.Model)
	}
	if resp.Text != "hi there" {
		t.Errorf("Expected 'hi there', got %s", resp.Text)
	}
}

func TestFailoverAgentBothFail(t *testing.T) {
	primary := &MockAgent{name: "primary", shouldErr: true}
	fallback := &MockAgent{name: "fallback", shouldErr: true}

	agent := NewFailoverAgent(primary, fallback)

	messages := []types.Message{{Text: "test"}}
	_, err := agent.Chat(messages)

	if err == nil {
		t.Error("Expected error when both agents fail")
	}
}

func TestFailoverAgentName(t *testing.T) {
	primary := &MockAgent{name: "primary"}
	fallback := &MockAgent{name: "fallback"}

	agent := NewFailoverAgent(primary, fallback)

	name := agent.Name()
	if name != "primary (with fallback: fallback)" {
		t.Errorf("Unexpected name: %s", name)
	}
}

func TestFailoverAgentStreamFallback(t *testing.T) {
	primary := &MockAgent{name: "primary", shouldErr: true}
	fallback := &MockAgent{name: "fallback", response: "streamed", shouldErr: false}

	agent := NewFailoverAgent(primary, fallback)

	messages := []types.Message{{Text: "test"}}
	resp, err := agent.ChatStream(messages, "", nil)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.Model != "fallback" {
		t.Errorf("Expected fallback model in stream, got %s", resp.Model)
	}
}
