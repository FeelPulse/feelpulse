package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
	defaultMaxTokens    = 4096
	claudeCodeVersion   = "1.0.33"
)

// AuthMode determines how to authenticate with Anthropic
type AuthMode int

const (
	AuthModeAPIKey AuthMode = iota
	AuthModeOAuth           // setup-token (subscription)
)

// AnthropicClient implements the Agent interface for Anthropic's Claude
type AnthropicClient struct {
	apiKey    string
	authToken string // OAuth setup-token
	authMode  AuthMode
	model     string
	client    *http.Client
}

// AnthropicRequest represents the request body for Claude API
type AnthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []AnthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
}

// AnthropicMessage represents a message in the Claude format
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicResponse represents the response from Claude API
type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Content      []AnthropicContent `json:"content"`
	Model        string             `json:"model"`
	StopReason   string             `json:"stop_reason"`
	StopSequence *string            `json:"stop_sequence"`
	Usage        AnthropicUsage     `json:"usage"`
}

// AnthropicContent represents a content block in the response
type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicUsage represents token usage info
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicError represents an API error response
type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type anthropicErrorWrapper struct {
	Error AnthropicError `json:"error"`
}

// IsOAuthToken checks if a token is an OAuth setup-token (subscription auth)
func IsOAuthToken(token string) bool {
	return strings.HasPrefix(token, "sk-ant-oat")
}

// NewAnthropicClient creates a new Anthropic Claude client.
// Pass apiKey for API key auth, or authToken for subscription (setup-token) auth.
// If both are provided, authToken takes priority.
func NewAnthropicClient(apiKey, authToken, model string) *AnthropicClient {
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	c := &AnthropicClient{
		apiKey:    apiKey,
		authToken: authToken,
		model:     model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}

	// Determine auth mode
	if authToken != "" && IsOAuthToken(authToken) {
		c.authMode = AuthModeOAuth
	} else if authToken != "" {
		// Treat non-oat tokens as API keys (fallback)
		c.apiKey = authToken
		c.authMode = AuthModeAPIKey
	} else {
		c.authMode = AuthModeAPIKey
	}

	return c
}

// Name returns the provider name
func (c *AnthropicClient) Name() string {
	return "anthropic"
}

// AuthModeName returns a human-readable auth mode description
func (c *AnthropicClient) AuthModeName() string {
	if c.authMode == AuthModeOAuth {
		return "subscription (setup-token)"
	}
	return "api-key"
}

// Chat sends messages to Claude and returns a response
func (c *AnthropicClient) Chat(messages []types.Message) (*types.AgentResponse, error) {
	// Convert our messages to Anthropic format
	anthropicMsgs := make([]AnthropicMessage, 0, len(messages))

	for _, msg := range messages {
		role := "user"
		if msg.IsBot {
			role = "assistant"
		}

		anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
			Role:    role,
			Content: msg.Text,
		})
	}

	// Build request
	reqBody := AnthropicRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  anthropicMsgs,
		System:    "You are a helpful AI assistant called FeelPulse. Be concise, friendly, and helpful.",
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest(http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	if c.authMode == AuthModeOAuth {
		// Subscription auth: mimic Claude Code headers
		req.Header.Set("Authorization", "Bearer "+c.authToken)
		req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		req.Header.Set("user-agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion))
		req.Header.Set("x-app", "cli")
		req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	} else {
		// Standard API key auth
		req.Header.Set("x-api-key", c.apiKey)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		var errResp anthropicErrorWrapper
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error: %s (%s)", errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("anthropic API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content blocks
	var text string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return &types.AgentResponse{
		Text:  text,
		Model: anthropicResp.Model,
		Usage: types.Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}, nil
}
