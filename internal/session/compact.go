package session

import (
	"time"
	"unicode/utf8"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	// DefaultMaxContextTokens is the default threshold for context compaction
	DefaultMaxContextTokens = 80000
	// DefaultKeepLastN is the default number of recent messages to keep intact
	DefaultKeepLastN = 10
)

// Summarizer interface for summarizing conversation history
type Summarizer interface {
	Summarize(messages []types.Message) (string, error)
}

// Compactor handles context compaction by summarizing old messages
type Compactor struct {
	summarizer Summarizer
	maxTokens  int
	keepLastN  int
}

// NewCompactor creates a new Compactor
func NewCompactor(summarizer Summarizer, maxTokens int, keepLastN int) *Compactor {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxContextTokens
	}
	if keepLastN <= 0 {
		keepLastN = DefaultKeepLastN
	}
	return &Compactor{
		summarizer: summarizer,
		maxTokens:  maxTokens,
		keepLastN:  keepLastN,
	}
}

// EstimateTokens estimates the token count for a string
// Better estimate: count runes (handles CJK properly)
// CJK characters ≈ 1-2 tokens each, ASCII ≈ 4 chars/token
func EstimateTokens(text string) int {
	runeCount := utf8.RuneCountInString(text)
	byteCount := len(text)
	// If mostly multi-byte (CJK), use rune count; otherwise bytes/4
	if byteCount > runeCount*2 {
		return runeCount // CJK-heavy: ~1 token per character
	}
	return byteCount / 4 // ASCII-heavy: ~4 chars per token
}

// EstimateHistoryTokens estimates total tokens in a message history
func EstimateHistoryTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg.Text)
	}
	return total
}

// NeedsCompaction returns true if the history exceeds the token threshold
func NeedsCompaction(messages []types.Message, maxTokens int) bool {
	return EstimateHistoryTokens(messages) > maxTokens
}

// SplitMessages splits messages into ones to summarize and ones to keep
func (c *Compactor) SplitMessages(messages []types.Message) (toSummarize, toKeep []types.Message) {
	if len(messages) <= c.keepLastN {
		// Keep all messages if we have fewer than keepLastN
		return nil, messages
	}

	splitIdx := len(messages) - c.keepLastN
	return messages[:splitIdx], messages[splitIdx:]
}

// CreateSummaryMessage creates a system message containing the summary
func CreateSummaryMessage(summary string) types.Message {
	return types.Message{
		Text:      summary,
		IsBot:     true,
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"type": "summary",
		},
	}
}

// CompactIfNeeded checks if compaction is needed and performs it
// Returns the compacted history or original if no compaction needed
func (c *Compactor) CompactIfNeeded(messages []types.Message) ([]types.Message, error) {
	if !NeedsCompaction(messages, c.maxTokens) {
		return messages, nil
	}

	return c.ForceCompact(messages)
}

// ForceCompact compacts the conversation regardless of token count
// Used by the /compact command
func (c *Compactor) ForceCompact(messages []types.Message) ([]types.Message, error) {
	toSummarize, toKeep := c.SplitMessages(messages)
	if len(toSummarize) == 0 {
		return messages, nil
	}

	// Get summary from the summarizer
	summary, err := c.summarizer.Summarize(toSummarize)
	if err != nil {
		// On error, return original messages (fail open)
		return messages, err
	}

	// Build new history: summary message + recent messages
	result := make([]types.Message, 0, 1+len(toKeep))
	result = append(result, CreateSummaryMessage(summary))
	result = append(result, toKeep...)

	return result, nil
}
