package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/tools"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// StreamCallback is called for each text delta during streaming
type StreamCallback func(delta string)

// Agent interface defines the contract for AI providers
type Agent interface {
	Chat(messages []types.Message) (*types.AgentResponse, error)
	ChatStream(messages []types.Message, systemPrompt string, callback StreamCallback) (*types.AgentResponse, error)
	Name() string
}

// SystemPromptBuilder builds the system prompt dynamically
type SystemPromptBuilder func(defaultPrompt string) string

// Router manages AI agent providers
type Router struct {
	cfg           *config.Config
	agent         Agent
	promptBuilder SystemPromptBuilder
	toolRegistry  *tools.Registry
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
	case "openai":
		if cfg.Agent.APIKey == "" {
			return nil, fmt.Errorf("openai API key not configured")
		}
		r.agent = NewOpenAIClient(cfg.Agent.APIKey, cfg.Agent.Model)
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
	return r.ProcessWithHistoryStream(messages, nil)
}

// ProcessWithHistoryStream handles messages with optional streaming callback
func (r *Router) ProcessWithHistoryStream(messages []types.Message, callback StreamCallback) (*types.Message, error) {
	if r.agent == nil {
		return nil, fmt.Errorf("no agent configured")
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	var resp *types.AgentResponse
	var err error

	// Get system prompt from config, optionally enhanced by prompt builder
	systemPrompt := r.cfg.Agent.System
	if r.promptBuilder != nil {
		systemPrompt = r.promptBuilder(systemPrompt)
	}

	// Check if we have tools and an Anthropic client - use agentic loop
	anthropicClient, isAnthropic := r.agent.(*AnthropicClient)
	if isAnthropic && r.toolRegistry != nil && len(r.toolRegistry.List()) > 0 {
		// Build Anthropic tool definitions
		anthropicTools := r.buildAnthropicTools()

		// Create tool executor
		executor := r.createToolExecutor()

		// Use agentic loop with tools
		resp, err = anthropicClient.ChatWithTools(messages, systemPrompt, anthropicTools, executor, 10, callback)
	} else if callback != nil {
		// Use streaming without tools
		resp, err = r.agent.ChatStream(messages, systemPrompt, callback)
	} else {
		// Use simple chat
		resp, err = r.agent.Chat(messages)
	}

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

// buildAnthropicTools converts registry tools to Anthropic format
func (r *Router) buildAnthropicTools() []AnthropicTool {
	registryTools := r.toolRegistry.List()
	anthropicTools := make([]AnthropicTool, 0, len(registryTools))

	for _, tool := range registryTools {
		schema := tool.ToAnthropicSchema()
		inputSchema, _ := json.Marshal(schema["input_schema"])

		anthropicTools = append(anthropicTools, AnthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		})
	}

	return anthropicTools
}

// createToolExecutor creates a function that executes tools from the registry
func (r *Router) createToolExecutor() ToolExecutor {
	return func(name string, input map[string]any) (string, error) {
		tool := r.toolRegistry.Get(name)
		if tool == nil {
			return "", fmt.Errorf("unknown tool: %s", name)
		}

		// TODO: pass parent context for graceful cancellation
		// Execute with a timeout context (60 seconds)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		return tool.Handler(ctx, input)
	}
}

// SystemPrompt returns the configured system prompt
func (r *Router) SystemPrompt() string {
	return r.cfg.Agent.System
}

// SetSystemPromptBuilder sets a function to build the system prompt dynamically
func (r *Router) SetSystemPromptBuilder(builder SystemPromptBuilder) {
	r.promptBuilder = builder
}

// Agent returns the current agent
func (r *Router) Agent() Agent {
	return r.agent
}

// SetToolRegistry sets the tool registry for agentic tool calling
func (r *Router) SetToolRegistry(registry *tools.Registry) {
	r.toolRegistry = registry
}

// ToolRegistry returns the current tool registry
func (r *Router) ToolRegistry() *tools.Registry {
	return r.toolRegistry
}
