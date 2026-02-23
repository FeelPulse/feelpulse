package session

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	// DefaultMaxContextTokens is the default threshold for context compaction
	DefaultMaxContextTokens = 80000
	// DefaultKeepRecentTokens is the default number of recent tokens to keep intact
	DefaultKeepRecentTokens = 15000
)

// CompactionDetails tracks file operations across compactions
type CompactionDetails struct {
	ReadFiles     []string `json:"readFiles"`
	ModifiedFiles []string `json:"modifiedFiles"`
}

// Summarizer interface for summarizing conversation history
type Summarizer interface {
	Summarize(messages []types.Message) (string, error)
}

// Compactor handles context compaction by summarizing old messages
type Compactor struct {
	summarizer       Summarizer
	maxTokens        int
	keepRecentTokens int
}

// NewCompactor creates a new Compactor
func NewCompactor(summarizer Summarizer, maxTokens int, keepRecentTokens int) *Compactor {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxContextTokens
	}
	if keepRecentTokens <= 0 {
		keepRecentTokens = DefaultKeepRecentTokens
	}
	return &Compactor{
		summarizer:       summarizer,
		maxTokens:        maxTokens,
		keepRecentTokens: keepRecentTokens,
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
// Uses token-based splitting: accumulates from newest messages backwards
// until keepRecentTokens threshold is reached
func (c *Compactor) SplitMessages(messages []types.Message) (toSummarize, toKeep []types.Message) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Accumulate tokens from newest to oldest
	keepTokens := 0
	splitIdx := -1 // -1 means "keep everything"

	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i].Text)
		
		// If adding this message would exceed budget, stop here
		if keepTokens+msgTokens > c.keepRecentTokens {
			splitIdx = i + 1
			break
		}
		
		keepTokens += msgTokens
	}

	// If splitIdx is still -1, we kept all messages (total < keepRecentTokens)
	if splitIdx == -1 {
		return nil, messages
	}

	// If we ended at index 0, keep everything
	if splitIdx == 0 {
		return nil, messages
	}

	// Split at the computed index
	return messages[:splitIdx], messages[splitIdx:]
}

// ExtractFileOpsFromMessages extracts file operations from message metadata
// and previous compaction details (cumulative tracking)
func ExtractFileOpsFromMessages(messages []types.Message) CompactionDetails {
	readSet := make(map[string]bool)
	modifiedSet := make(map[string]bool)

	for _, msg := range messages {
		// Check if this is a previous summary with compaction details
		if msg.Metadata != nil {
			if msg.Metadata["type"] == "summary" {
				// Extract from previous compaction
				if details, ok := msg.Metadata["compactionDetails"].(map[string]any); ok {
					if readFiles, ok := details["readFiles"].([]any); ok {
						for _, f := range readFiles {
							if path, ok := f.(string); ok {
								readSet[path] = true
							}
						}
					}
					if modFiles, ok := details["modifiedFiles"].([]any); ok {
						for _, f := range modFiles {
							if path, ok := f.(string); ok {
								modifiedSet[path] = true
							}
						}
					}
				}
			}

			// Extract from tool call metadata (if present)
			if toolName, ok := msg.Metadata["tool"].(string); ok {
				if path, ok := msg.Metadata["path"].(string); ok {
					switch toolName {
					case "file_read", "file_list":
						readSet[path] = true
					case "file_write":
						modifiedSet[path] = true
					}
				}
			}
		}
	}

	// Convert sets to sorted slices
	readFiles := make([]string, 0, len(readSet))
	for f := range readSet {
		readFiles = append(readFiles, f)
	}

	modifiedFiles := make([]string, 0, len(modifiedSet))
	for f := range modifiedSet {
		modifiedFiles = append(modifiedFiles, f)
	}

	return CompactionDetails{
		ReadFiles:     readFiles,
		ModifiedFiles: modifiedFiles,
	}
}

// AppendFileListsToSummary appends file lists to summary in structured format
func AppendFileListsToSummary(summary string, details CompactionDetails) string {
	var sb strings.Builder
	sb.WriteString(summary)

	if len(details.ReadFiles) > 0 {
		sb.WriteString("\n\n<read-files>\n")
		for _, f := range details.ReadFiles {
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("</read-files>")
	}

	if len(details.ModifiedFiles) > 0 {
		sb.WriteString("\n\n<modified-files>\n")
		for _, f := range details.ModifiedFiles {
			sb.WriteString(f)
			sb.WriteString("\n")
		}
		sb.WriteString("</modified-files>")
	}

	return sb.String()
}

// CreateSummaryMessage creates a system message containing the summary
func CreateSummaryMessage(summary string, details CompactionDetails) types.Message {
	// Append file lists to summary text
	summaryWithFiles := AppendFileListsToSummary(summary, details)

	return types.Message{
		Text:      summaryWithFiles,
		IsBot:     true,
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"type": "summary",
			"compactionDetails": map[string]any{
				"readFiles":     details.ReadFiles,
				"modifiedFiles": details.ModifiedFiles,
			},
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

	// Extract file operations from messages being summarized
	// This accumulates file tracking from previous compactions
	fileOps := ExtractFileOpsFromMessages(toSummarize)

	// Get summary from the summarizer
	summary, err := c.summarizer.Summarize(toSummarize)
	if err != nil {
		// On error, return original messages (fail open)
		return messages, err
	}

	// Build new history: summary message + recent messages
	result := make([]types.Message, 0, 1+len(toKeep))
	result = append(result, CreateSummaryMessage(summary, fileOps))
	result = append(result, toKeep...)

	return result, nil
}
