package agent

import (
	"fmt"
	"strings"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const summarySystemPrompt = `You are a conversation summarizer. Summarize the following conversation history concisely, preserving key facts, decisions, and context that would be important for continuing the conversation. Focus on:
- Main topics discussed
- Important information shared
- Any decisions or conclusions reached
- User preferences or requests mentioned

Keep the summary brief but comprehensive. Write in third person (e.g., "The user asked about..." "The assistant explained...").`

// ConversationSummarizer uses an AI agent to summarize conversation history
type ConversationSummarizer struct {
	client *AnthropicClient
}

// NewConversationSummarizer creates a new summarizer using the given Anthropic client
func NewConversationSummarizer(client *AnthropicClient) *ConversationSummarizer {
	return &ConversationSummarizer{client: client}
}

// Summarize condenses multiple messages into a single summary
func (s *ConversationSummarizer) Summarize(messages []types.Message) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}

	// Build conversation text for summarization
	var sb strings.Builder
	sb.WriteString("Summarize this conversation:\n\n")

	for _, msg := range messages {
		role := "User"
		if msg.IsBot {
			role = "Assistant"
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", role, msg.Text))
	}

	// Create a single message asking for summary
	summaryRequest := []types.Message{
		{
			Text:  sb.String(),
			IsBot: false,
		},
	}

	// Call the AI to summarize
	resp, err := s.client.ChatWithSystem(summaryRequest, summarySystemPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to summarize: %w", err)
	}

	return resp.Text, nil
}
