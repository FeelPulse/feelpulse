package agent

import (
	"fmt"
	"strings"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const summarySystemPrompt = `You are a conversation summarizer. Summarize the following conversation history in a STRUCTURED FORMAT.

Use this exact template:

## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Requirements or preferences mentioned by user]

## Progress
### Done
- [x] [Completed tasks or topics covered]

### In Progress
- [ ] [Current work or ongoing discussions]

### Blocked
- [Issues or blockers, if any]

## Key Decisions
- **[Decision topic]**: [Rationale or outcome]

## Next Steps
1. [What should happen next to continue the conversation]

## Critical Context
- [Important data, facts, or state needed to continue]

RULES:
- Use third person (e.g., "User asked...", "Assistant explained...")
- Be concise but preserve essential details
- Focus on facts, decisions, and actionable context
- If a section is empty, write "None" instead of omitting it`

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
