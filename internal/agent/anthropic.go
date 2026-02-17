package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	Stream    bool               `json:"stream,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicTool represents a tool definition for Claude
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// SSEEvent represents a Server-Sent Event from the streaming API
type SSEEvent struct {
	Type    string         `json:"type"`
	Index   int            `json:"index,omitempty"`
	Delta   SSEDelta       `json:"delta,omitempty"`
	Message *SSEMessage    `json:"message,omitempty"`
	Usage   *AnthropicUsage `json:"usage,omitempty"`
}

// SSEDelta represents a text delta in streaming
type SSEDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

// SSEMessage represents message info in streaming
type SSEMessage struct {
	ID    string         `json:"id,omitempty"`
	Model string         `json:"model,omitempty"`
	Usage *AnthropicUsage `json:"usage,omitempty"`
}

// AnthropicMessage represents a message in the Claude format
// Content can be a string or an array of content blocks
type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in messages (for tool use/results/images)
type ContentBlock struct {
	Type      string          `json:"type"`                  // "text", "tool_use", "tool_result", "image"
	Text      string          `json:"text,omitempty"`        // for type="text"
	ID        string          `json:"id,omitempty"`          // for type="tool_use"
	Name      string          `json:"name,omitempty"`        // for type="tool_use"
	Input     json.RawMessage `json:"input,omitempty"`       // for type="tool_use"
	ToolUseID string          `json:"tool_use_id,omitempty"` // for type="tool_result"
	Content   string          `json:"content,omitempty"`     // for type="tool_result" (result text)
	Source    *ImageSource    `json:"source,omitempty"`      // for type="image"
}

// ImageSource represents an image source for vision
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg", "image/png", etc.
	Data      string `json:"data"`       // base64-encoded image data
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
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`    // for tool_use
	Name  string          `json:"name,omitempty"`  // for tool_use
	Input json.RawMessage `json:"input,omitempty"` // for tool_use
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

// setHeaders sets the common HTTP headers for Anthropic API requests
func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if c.authMode == AuthModeOAuth {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
		req.Header.Set("anthropic-beta", "claude-code-20250219,oauth-2025-04-20")
		req.Header.Set("user-agent", fmt.Sprintf("claude-cli/%s (external, cli)", claudeCodeVersion))
		req.Header.Set("x-app", "cli")
		req.Header.Set("anthropic-dangerous-direct-browser-access", "true")
	} else {
		req.Header.Set("x-api-key", c.apiKey)
	}
}

// DefaultSystemPrompt is the default system prompt for FeelPulse
const DefaultSystemPrompt = "You are a helpful AI assistant called FeelPulse. Be concise, friendly, and helpful."

// convertMessagesToAnthropic converts types.Message to AnthropicMessage format
// Handles both text and image content
func convertMessagesToAnthropic(messages []types.Message) []AnthropicMessage {
	anthropicMsgs := make([]AnthropicMessage, 0, len(messages))

	for _, msg := range messages {
		role := "user"
		if msg.IsBot {
			role = "assistant"
		}

		// Check if message has image data
		if msg.Metadata != nil {
			if imageData, ok := msg.Metadata["image"].(map[string]string); ok {
				data := imageData["data"]
				mediaType := imageData["media_type"]
				if data != "" && mediaType != "" {
					// Create multi-part content with image and text
					content := []ContentBlock{
						{
							Type: "image",
							Source: &ImageSource{
								Type:      "base64",
								MediaType: mediaType,
								Data:      data,
							},
						},
						{
							Type: "text",
							Text: msg.Text,
						},
					}
					anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
						Role:    role,
						Content: content,
					})
					continue
				}
			}
		}

		// Regular text message
		anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
			Role:    role,
			Content: msg.Text,
		})
	}

	return anthropicMsgs
}

// Chat sends messages to Claude and returns a response (uses default system prompt)
func (c *AnthropicClient) Chat(messages []types.Message) (*types.AgentResponse, error) {
	return c.ChatWithSystem(messages, "")
}

// ChatWithSystem sends messages to Claude with a custom system prompt
func (c *AnthropicClient) ChatWithSystem(messages []types.Message, systemPrompt string) (*types.AgentResponse, error) {
	// Convert our messages to Anthropic format
	anthropicMsgs := convertMessagesToAnthropic(messages)

	// Use default system prompt if not provided
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	// Build request
	reqBody := AnthropicRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  anthropicMsgs,
		System:    systemPrompt,
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

	c.setHeaders(req)

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

	log.Printf("📥 [anthropic] response received (%d chars)", len(text))

	return &types.AgentResponse{
		Text:  text,
		Model: anthropicResp.Model,
		Usage: types.Usage{
			InputTokens:  anthropicResp.Usage.InputTokens,
			OutputTokens: anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

// ChatStream sends messages to Claude with streaming and calls callback for each delta
func (c *AnthropicClient) ChatStream(messages []types.Message, systemPrompt string, callback StreamCallback) (*types.AgentResponse, error) {
	// Convert our messages to Anthropic format
	anthropicMsgs := convertMessagesToAnthropic(messages)

	// Use default system prompt if not provided
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant called FeelPulse. Be concise, friendly, and helpful."
	}

	// Build request with streaming enabled
	reqBody := AnthropicRequest{
		Model:     c.model,
		MaxTokens: defaultMaxTokens,
		Messages:  anthropicMsgs,
		System:    systemPrompt,
		Stream:    true,
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

	c.setHeaders(req)

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp anthropicErrorWrapper
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error: %s (%s)", errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("anthropic API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse SSE stream
	var fullText strings.Builder
	var model string
	var usage types.Usage

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data lines
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Skip [DONE] marker
			if data == "[DONE]" {
				continue
			}

			event, err := parseSSEEvent(data)
			if err != nil {
				log.Printf("⚠️ Failed to parse SSE event: %v", err)
				continue
			}

			switch event.Type {
			case "message_start":
				if event.Message != nil {
					model = event.Message.Model
					if event.Message.Usage != nil {
						usage.InputTokens = event.Message.Usage.InputTokens
					}
				}
			case "content_block_delta":
				if event.Delta.Text != "" {
					fullText.WriteString(event.Delta.Text)
					if callback != nil {
						callback(event.Delta.Text)
					}
				}
			case "message_delta":
				if event.Usage != nil {
					usage.OutputTokens = event.Usage.OutputTokens
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	text := fullText.String()
	log.Printf("📥 [anthropic/stream] response received (%d chars)", len(text))

	return &types.AgentResponse{
		Text:  text,
		Model: model,
		Usage: usage,
	}, nil
}

// parseSSEEvent parses a JSON SSE event
func parseSSEEvent(data string) (*SSEEvent, error) {
	var event SSEEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil, fmt.Errorf("failed to parse SSE event: %w", err)
	}
	return &event, nil
}

// ToolExecutor is a function that executes a tool and returns the result
type ToolExecutor func(name string, input map[string]any) (string, error)

// ChatWithTools sends messages to Claude with tools and implements the full agentic loop.
// It will call tools as requested by Claude and continue the conversation until done.
// maxIterations prevents infinite loops (default 10 if <= 0).
func (c *AnthropicClient) ChatWithTools(
	messages []types.Message,
	systemPrompt string,
	tools []AnthropicTool,
	executor ToolExecutor,
	maxIterations int,
	callback StreamCallback,
) (*types.AgentResponse, error) {
	if maxIterations <= 0 {
		maxIterations = 10
	}

	// Convert our messages to Anthropic format
	anthropicMsgs := convertMessagesToAnthropic(messages)

	// Use default system prompt if not provided
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	var totalUsage types.Usage
	var finalText strings.Builder
	var model string

	for iteration := 0; iteration < maxIterations; iteration++ {
		// Build request
		reqBody := AnthropicRequest{
			Model:     c.model,
			MaxTokens: defaultMaxTokens,
			Messages:  anthropicMsgs,
			System:    systemPrompt,
			Tools:     tools,
		}

		resp, err := c.callAPI(reqBody)
		if err != nil {
			return nil, err
		}

		model = resp.Model
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		// Extract text and tool_use blocks from response
		var textContent strings.Builder
		var toolUseBlocks []ContentBlock

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				textContent.WriteString(block.Text)
				if callback != nil {
					callback(block.Text)
				}
			case "tool_use":
				toolUseBlocks = append(toolUseBlocks, ContentBlock{
					Type:  "tool_use",
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				})
			}
		}

		finalText.WriteString(textContent.String())

		// If stop_reason is not "tool_use", we're done
		if resp.StopReason != "tool_use" || len(toolUseBlocks) == 0 {
			log.Printf("📥 [anthropic/agentic] final response (iteration %d, %d chars)", iteration+1, finalText.Len())
			break
		}

		// Add assistant message with tool_use blocks to conversation
		anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
			Role:    "assistant",
			Content: resp.Content, // Include all content blocks
		})

		// Execute each tool and collect results
		var toolResults []ContentBlock
		for _, toolUse := range toolUseBlocks {
			// Parse input JSON
			var input map[string]any
			if err := json.Unmarshal(toolUse.Input, &input); err != nil {
				input = make(map[string]any)
			}

			// Execute tool
			log.Printf("🔧 [tool] executing %s", toolUse.Name)

			result, err := executor(toolUse.Name, input)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				log.Printf("🔧 [tool] %s → error: %v", toolUse.Name, err)
			} else {
				log.Printf("🔧 [tool] %s → success (%d chars)", toolUse.Name, len(result))
			}

			toolResults = append(toolResults, ContentBlock{
				Type:      "tool_result",
				ToolUseID: toolUse.ID,
				Content:   result,
			})
		}

		// Add user message with tool results
		anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
			Role:    "user",
			Content: toolResults,
		})
	}

	return &types.AgentResponse{
		Text:  finalText.String(),
		Model: model,
		Usage: totalUsage,
	}, nil
}

// callAPI makes a single API call to Anthropic (non-streaming)
func (c *AnthropicClient) callAPI(reqBody AnthropicRequest) (*AnthropicResponse, error) {
	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp anthropicErrorWrapper
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("anthropic API error: %s (%s)", errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("anthropic API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &anthropicResp, nil
}

// truncateString truncates a string to maxLen and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// truncateJSON truncates JSON to maxLen for logging
func truncateJSON(data json.RawMessage, maxLen int) string {
	s := string(data)
	return truncateString(s, maxLen)
}
