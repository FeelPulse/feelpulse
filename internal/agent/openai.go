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
	openaiAPIURL       = "https://api.openai.com/v1/chat/completions"
	defaultOpenAIModel = "gpt-4o"
)

// OpenAIClient implements the Agent interface for OpenAI
type OpenAIClient struct {
	apiKey string
	model  string
	client *http.Client
}

// OpenAIRequest represents the request body for OpenAI Chat API
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIResponse represents the response from OpenAI Chat API
type OpenAIResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChoice   `json:"choices"`
	Usage   OpenAIUsage      `json:"usage"`
	Error   *OpenAIErrorInfo `json:"error,omitempty"`
}

// OpenAIChoice represents a choice in the response
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	Delta        OpenAIMessage `json:"delta,omitempty"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIUsage represents token usage info
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIErrorInfo represents an error from the API
type OpenAIErrorInfo struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(apiKey, model string) *OpenAIClient {
	if model == "" {
		model = defaultOpenAIModel
	}

	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Name returns the provider name
func (c *OpenAIClient) Name() string {
	return "openai"
}

// convertMessages converts internal messages to OpenAI format
func (c *OpenAIClient) convertMessages(messages []types.Message) []OpenAIMessage {
	openaiMsgs := make([]OpenAIMessage, 0, len(messages))

	for _, msg := range messages {
		role := "user"
		if msg.IsBot {
			role = "assistant"
		}

		openaiMsgs = append(openaiMsgs, OpenAIMessage{
			Role:    role,
			Content: msg.Text,
		})
	}

	return openaiMsgs
}

// Chat sends messages to OpenAI and returns a response
func (c *OpenAIClient) Chat(messages []types.Message) (*types.AgentResponse, error) {
	return c.ChatWithSystem(messages, "")
}

// ChatWithSystem sends messages with a custom system prompt
func (c *OpenAIClient) ChatWithSystem(messages []types.Message, systemPrompt string) (*types.AgentResponse, error) {
	openaiMsgs := c.convertMessages(messages)

	// Prepend system message if provided
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	openaiMsgs = append([]OpenAIMessage{{Role: "system", Content: systemPrompt}}, openaiMsgs...)

	reqBody := OpenAIRequest{
		Model:     c.model,
		Messages:  openaiMsgs,
		MaxTokens: defaultMaxTokens,
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, openaiAPIURL, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var openaiResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if openaiResp.Error != nil {
		return nil, fmt.Errorf("openai API error: %s (%s)", openaiResp.Error.Message, openaiResp.Error.Type)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	text := openaiResp.Choices[0].Message.Content
	log.Printf("📥 [openai] response: %s", text)

	return &types.AgentResponse{
		Text:  text,
		Model: openaiResp.Model,
		Usage: types.Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
	}, nil
}

// ChatStream sends messages with streaming and calls callback for each delta
func (c *OpenAIClient) ChatStream(messages []types.Message, systemPrompt string, callback StreamCallback) (*types.AgentResponse, error) {
	openaiMsgs := c.convertMessages(messages)

	// Prepend system message if provided
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	openaiMsgs = append([]OpenAIMessage{{Role: "system", Content: systemPrompt}}, openaiMsgs...)

	reqBody := OpenAIRequest{
		Model:     c.model,
		Messages:  openaiMsgs,
		MaxTokens: defaultMaxTokens,
		Stream:    true,
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, openaiAPIURL, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var openaiResp OpenAIResponse
		if json.Unmarshal(respBody, &openaiResp) == nil && openaiResp.Error != nil {
			return nil, fmt.Errorf("openai API error: %s (%s)", openaiResp.Error.Message, openaiResp.Error.Type)
		}
		return nil, fmt.Errorf("openai API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse SSE stream
	var fullText strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		delta, done, err := parseOpenAISSE(data)
		if err != nil {
			log.Printf("⚠️ Failed to parse OpenAI SSE: %v", err)
			continue
		}

		if done {
			break
		}

		if delta != "" {
			fullText.WriteString(delta)
			if callback != nil {
				callback(delta)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	text := fullText.String()
	log.Printf("📥 [openai/stream] response: %s", text)

	return &types.AgentResponse{
		Text:  text,
		Model: c.model,
		Usage: types.Usage{}, // Usage not available in streaming mode
	}, nil
}

// parseOpenAISSE parses an OpenAI SSE data line
func parseOpenAISSE(data string) (delta string, done bool, err error) {
	if data == "[DONE]" {
		return "", true, nil
	}

	var chunk OpenAIResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", false, fmt.Errorf("failed to parse SSE: %w", err)
	}

	if len(chunk.Choices) > 0 {
		return chunk.Choices[0].Delta.Content, false, nil
	}

	return "", false, nil
}
