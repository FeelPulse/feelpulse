package types

import "time"

// Message represents a chat message
type Message struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	From      string         `json:"from"`
	Channel   string         `json:"channel"`
	Timestamp time.Time      `json:"timestamp"`
	IsBot     bool           `json:"isBot"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// AgentRequest is sent to the AI model
type AgentRequest struct {
	Messages []Message `json:"messages"`
	Model    string    `json:"model"`
	Provider string    `json:"provider"`
}

// AgentResponse is received from the AI model
type AgentResponse struct {
	Text  string `json:"text"`
	Model string `json:"model"`
	Usage Usage  `json:"usage"`
}

// Usage tracks token consumption
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}
