package agent

import (
	"fmt"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Agent interface defines the contract for AI providers
type Agent interface {
	Chat(messages []types.Message) (*types.AgentResponse, error)
	Name() string
}

// Router manages AI agent providers
type Router struct {
	cfg     *config.Config
	agent   Agent
}

// NewRouter creates a new agent router
func NewRouter(cfg *config.Config) (*Router, error) {
	r := &Router{cfg: cfg}

	// Initialize the configured provider
	switch cfg.Agent.Provider {
	case "anthropic", "":
		if cfg.Agent.APIKey == "" && cfg.Agent.AuthToken == "" {
			return nil, fmt.Errorf("anthropic credentials not configured (set apiKey or authToken)")
		}
		r.agent = NewAnthropicClient(cfg.Agent.APIKey, cfg.Agent.AuthToken, cfg.Agent.Model)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Agent.Provider)
	}

	return r, nil
}

// Process handles a single message and returns a response (no history)
func (r *Router) Process(msg *types.Message) (*types.Message, error) {
	return r.ProcessWithHistory([]types.Message{*msg})
}

// ProcessWithHistory handles a message with full conversation history
func (r *Router) ProcessWithHistory(messages []types.Message) (*types.Message, error) {
	if r.agent == nil {
		return nil, fmt.Errorf("no agent configured")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Call the agent with full history
	resp, err := r.agent.Chat(messages)
	if err != nil {
		return nil, fmt.Errorf("agent error: %w", err)
	}

	// Get channel from last message
	channel := messages[len(messages)-1].Channel

	// Create response message
	reply := &types.Message{
		Text:    resp.Text,
		Channel: channel,
		IsBot:   true,
		Metadata: map[string]any{
			"model":         resp.Model,
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
		},
	}

	return reply, nil
}

// Agent returns the current agent
func (r *Router) Agent() Agent {
	return r.agent
}
