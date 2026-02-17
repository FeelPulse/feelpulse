package channel

import (
	"strings"
	"unicode"
)

const (
	// TelegramMaxMessageLength is Telegram's max message length
	TelegramMaxMessageLength = 4096
	// SafeMessageLength leaves room for continuation indicators
	SafeMessageLength = 4000
)

// SplitLongMessage splits a message into chunks that fit Telegram's limit.
// It tries to split at paragraph boundaries, then sentence boundaries,
// then word boundaries, to avoid breaking markdown formatting.
func SplitLongMessage(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = SafeMessageLength
	}

	if len(text) <= maxLen {
		return []string{text}
	}

	var parts []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			parts = append(parts, remaining)
			break
		}

		// Find a good split point
		splitPoint := findSplitPoint(remaining, maxLen)
		
		part := strings.TrimSpace(remaining[:splitPoint])
		if len(parts) > 0 {
			// Not the first part - no indicator needed at start
		}
		
		// Add continuation indicator if this isn't the last part
		if len(remaining) > splitPoint {
			part += "\n\n_(continued...)_"
		}
		
		parts = append(parts, part)
		remaining = strings.TrimSpace(remaining[splitPoint:])
	}

	return parts
}

// findSplitPoint finds the best point to split a message.
// Tries paragraph > sentence > word boundaries in that order.
func findSplitPoint(text string, maxLen int) int {
	if len(text) <= maxLen {
		return len(text)
	}

	// Leave room for continuation indicator
	searchEnd := maxLen - 30
	if searchEnd < maxLen/2 {
		searchEnd = maxLen - 10
	}

	searchText := text[:searchEnd]

	// Try to split at a paragraph boundary (double newline)
	if idx := strings.LastIndex(searchText, "\n\n"); idx > maxLen/2 {
		return idx + 2 // Include the newlines
	}

	// Try to split at a single newline
	if idx := strings.LastIndex(searchText, "\n"); idx > maxLen/2 {
		return idx + 1
	}

	// Try to split at sentence boundary (. ! ?)
	if idx := findLastSentenceEnd(searchText); idx > maxLen/2 {
		return idx + 1
	}

	// Try to split at a word boundary (space)
	if idx := strings.LastIndex(searchText, " "); idx > maxLen/2 {
		return idx + 1
	}

	// Fallback: hard split at maxLen
	return maxLen
}

// findLastSentenceEnd finds the last sentence-ending punctuation followed by space or EOL
func findLastSentenceEnd(text string) int {
	bestIdx := -1
	runes := []rune(text)
	
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		if r == '.' || r == '!' || r == '?' {
			// Check if followed by space or end of text
			if i == len(runes)-1 {
				bestIdx = i
				break
			}
			next := runes[i+1]
			if unicode.IsSpace(next) {
				bestIdx = i
				break
			}
		}
	}
	
	if bestIdx >= 0 {
		// Convert rune index back to byte index
		return len(string(runes[:bestIdx+1]))
	}
	return -1
}

// TruncateForPreview truncates text for preview display (e.g., history)
func TruncateForPreview(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	
	// Try to truncate at word boundary
	truncated := text[:maxLen-3]
	if idx := strings.LastIndex(truncated, " "); idx > maxLen/2 {
		return truncated[:idx] + "..."
	}
	
	return truncated + "..."
}
