package agent

import (
	"fmt"
	"log"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// FailoverAgent wraps two agents and falls back to the second if the first fails
type FailoverAgent struct {
	primary  Agent
	fallback Agent
}

// NewFailoverAgent creates a new failover agent
func NewFailoverAgent(primary, fallback Agent) *FailoverAgent {
	return &FailoverAgent{
		primary:  primary,
		fallback: fallback,
	}
}

// Name returns the agent name
func (f *FailoverAgent) Name() string {
	return fmt.Sprintf("%s (with fallback: %s)", f.primary.Name(), f.fallback.Name())
}

// Chat sends messages to the primary agent, falling back if it fails
func (f *FailoverAgent) Chat(messages []types.Message) (*types.AgentResponse, error) {
	// Try primary first
	resp, err := f.primary.Chat(messages)
	if err == nil {
		return resp, nil
	}

	log.Printf("⚠️ Primary agent (%s) failed: %v, trying fallback (%s)", f.primary.Name(), err, f.fallback.Name())

	// Try fallback
	resp, err = f.fallback.Chat(messages)
	if err != nil {
		return nil, fmt.Errorf("both primary and fallback failed: %w", err)
	}

	return resp, nil
}

// ChatStream sends messages with streaming, falling back if primary fails
func (f *FailoverAgent) ChatStream(messages []types.Message, systemPrompt string, callback StreamCallback) (*types.AgentResponse, error) {
	// Try primary first
	resp, err := f.primary.ChatStream(messages, systemPrompt, callback)
	if err == nil {
		return resp, nil
	}

	log.Printf("⚠️ Primary agent (%s) streaming failed: %v, trying fallback (%s)", f.primary.Name(), err, f.fallback.Name())

	// Try fallback
	resp, err = f.fallback.ChatStream(messages, systemPrompt, callback)
	if err != nil {
		return nil, fmt.Errorf("both primary and fallback failed: %w", err)
	}

	return resp, nil
}
