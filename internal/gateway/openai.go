package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// OpenAI API compatible types

// OpenAIRequest represents an OpenAI chat completion request
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	User        string          `json:"user,omitempty"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIResponse represents an OpenAI chat completion response
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// OpenAIChoice represents a choice in the response
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// OpenAIUsage represents token usage in OpenAI format
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIError represents an OpenAI API error
type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// OpenAIErrorResponse wraps an error
type OpenAIErrorResponse struct {
	Error OpenAIError `json:"error"`
}

// handleOpenAIChatCompletion handles POST /v1/chat/completions
func (gw *Gateway) handleOpenAIChatCompletion(w http.ResponseWriter, r *http.Request) {
	// Check method
	if r.Method != http.MethodPost {
		gw.writeOpenAIError(w, http.StatusMethodNotAllowed, "Method not allowed", "invalid_request_error")
		return
	}

	// Check auth
	if gw.cfg.Hooks.Token != "" {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			gw.writeOpenAIError(w, http.StatusUnauthorized, "Missing or invalid Authorization header", "invalid_request_error")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != gw.cfg.Hooks.Token {
			gw.writeOpenAIError(w, http.StatusUnauthorized, "Invalid API key", "invalid_request_error")
			return
		}
	}

	// Parse request
	var req OpenAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		gw.writeOpenAIError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error(), "invalid_request_error")
		return
	}

	// Validate request
	if len(req.Messages) == 0 {
		gw.writeOpenAIError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	// Check if streaming is requested (not supported yet)
	if req.Stream {
		gw.writeOpenAIError(w, http.StatusBadRequest, "Streaming is not supported", "invalid_request_error")
		return
	}

	// Get router
	gw.mu.RLock()
	router := gw.router
	gw.mu.RUnlock()

	if router == nil {
		gw.writeOpenAIError(w, http.StatusServiceUnavailable, "AI agent not configured", "server_error")
		return
	}

	// Convert OpenAI messages to internal format
	messages, systemPrompt := convertOpenAIToInternal(&req)

	// Map model name if it's an OpenAI model
	model := mapToAnthropicModel(req.Model)
	log.Printf("📡 OpenAI API: model=%s → %s, messages=%d", req.Model, model, len(messages))

	// Process with the agent
	reply, err := router.ProcessWithHistory(messages)
	if err != nil {
		log.Printf("❌ OpenAI API error: %v", err)
		gw.writeOpenAIError(w, http.StatusInternalServerError, "Failed to process request: "+err.Error(), "server_error")
		return
	}

	// Extract usage from reply metadata
	var inputTokens, outputTokens int
	if reply.Metadata != nil {
		if v, ok := reply.Metadata["input_tokens"].(int); ok {
			inputTokens = v
		}
		if v, ok := reply.Metadata["output_tokens"].(int); ok {
			outputTokens = v
		}
	}

	// Build response
	resp := OpenAIResponse{
		ID:      generateCompletionID(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: reply.Text,
				},
				FinishReason: "stop",
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		},
	}

	// Log for debugging
	_ = systemPrompt // Used internally by router

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// writeOpenAIError writes an error response in OpenAI format
func (gw *Gateway) writeOpenAIError(w http.ResponseWriter, status int, message, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(OpenAIErrorResponse{
		Error: OpenAIError{
			Message: message,
			Type:    errType,
		},
	})
}

// convertOpenAIToInternal converts OpenAI messages to internal format
func convertOpenAIToInternal(req *OpenAIRequest) ([]types.Message, string) {
	var systemPrompt string
	var messages []types.Message

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}

		messages = append(messages, types.Message{
			Text:      msg.Content,
			Channel:   "api",
			IsBot:     msg.Role == "assistant",
			Timestamp: time.Now(),
		})
	}

	return messages, systemPrompt
}

// generateCompletionID generates a unique completion ID
func generateCompletionID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}

// mapToAnthropicModel maps OpenAI model names to Anthropic equivalents
func mapToAnthropicModel(model string) string {
	switch {
	case strings.HasPrefix(model, "gpt-4"):
		return "claude-sonnet-4-20250514"
	case strings.HasPrefix(model, "gpt-3.5"):
		return "claude-3-haiku-20240307"
	case strings.HasPrefix(model, "claude"):
		return model // Already an Anthropic model
	default:
		return "claude-sonnet-4-20250514" // Default
	}
}
