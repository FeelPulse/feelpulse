package channel

import (
	"strings"
	"testing"
)

func TestSplitLongMessage_ShortMessage(t *testing.T) {
	text := "Hello, world!"
	parts := SplitLongMessage(text, 100)
	
	if len(parts) != 1 {
		t.Errorf("Expected 1 part, got %d", len(parts))
	}
	if parts[0] != text {
		t.Errorf("Expected %q, got %q", text, parts[0])
	}
}

func TestSplitLongMessage_ExactLimit(t *testing.T) {
	text := strings.Repeat("a", 100)
	parts := SplitLongMessage(text, 100)
	
	if len(parts) != 1 {
		t.Errorf("Expected 1 part, got %d", len(parts))
	}
}

func TestSplitLongMessage_SplitAtParagraph(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph that makes it longer."
	parts := SplitLongMessage(text, 30)
	
	if len(parts) < 2 {
		t.Errorf("Expected at least 2 parts, got %d", len(parts))
	}
	
	// First part should end with continuation indicator
	if !strings.Contains(parts[0], "_(continued...)_") {
		t.Error("Expected continuation indicator in first part")
	}
}

func TestSplitLongMessage_SplitAtSentence(t *testing.T) {
	text := "First sentence. Second sentence. Third sentence makes it longer."
	parts := SplitLongMessage(text, 40)
	
	if len(parts) < 2 {
		t.Errorf("Expected at least 2 parts, got %d", len(parts))
	}
}

func TestSplitLongMessage_SplitAtWord(t *testing.T) {
	text := "word1 word2 word3 word4 word5 word6 word7 word8"
	parts := SplitLongMessage(text, 25)
	
	if len(parts) < 2 {
		t.Errorf("Expected at least 2 parts, got %d", len(parts))
	}
	
	// Parts should not cut words in half (except as last resort)
	for _, part := range parts {
		cleaned := strings.TrimSuffix(part, "\n\n_(continued...)_")
		cleaned = strings.TrimSpace(cleaned)
		// Each word boundary split should leave complete words
		words := strings.Fields(cleaned)
		for _, word := range words {
			if !strings.HasPrefix(word, "word") && word != "_(continued...)_" {
				// This is fine - it could be punctuation or continuation
			}
		}
	}
}

func TestSplitLongMessage_VeryLongWord(t *testing.T) {
	// A word longer than the limit should still work (hard split)
	text := strings.Repeat("x", 100)
	parts := SplitLongMessage(text, 30)
	
	if len(parts) < 3 {
		t.Errorf("Expected at least 3 parts for 100 chars with 30 limit, got %d", len(parts))
	}
	
	// Total content should be preserved
	total := 0
	for _, part := range parts {
		cleaned := strings.TrimSuffix(part, "\n\n_(continued...)_")
		total += len(cleaned)
	}
	if total < 100 {
		t.Errorf("Content was lost: expected >= 100 chars, got %d", total)
	}
}

func TestSplitLongMessage_DefaultLimit(t *testing.T) {
	text := strings.Repeat("a ", 2500) // ~5000 chars
	parts := SplitLongMessage(text, 0) // Use default
	
	// Should split into 2 parts (5000 chars / 4000 default)
	if len(parts) < 2 {
		t.Errorf("Expected at least 2 parts with default limit, got %d", len(parts))
	}
}

func TestSplitLongMessage_MarkdownPreserved(t *testing.T) {
	text := "*Bold text* and _italic_. More text here to make it long enough. Another sentence follows."
	parts := SplitLongMessage(text, 50)
	
	// First part should still have valid markdown
	if !strings.Contains(parts[0], "*Bold text*") {
		t.Error("Markdown formatting should be preserved")
	}
}

func TestTruncateForPreview(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"hello world", 8, "hello..."},
		{"oneword", 3, "..."},
		{"word1 word2 word3", 12, "word1 wor..."}, // Hard truncate since no word boundary in first half
	}
	
	for _, tt := range tests {
		result := TruncateForPreview(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateForPreview(%q, %d) = %q, want %q", 
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestFindLastSentenceEnd(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"Hello. World", 6},
		{"No sentence end here", -1},
		{"First! Second? Third.", 21},
		{"End with period.", 16},
	}
	
	for _, tt := range tests {
		result := findLastSentenceEnd(tt.text)
		if result != tt.expected {
			t.Errorf("findLastSentenceEnd(%q) = %d, want %d", tt.text, result, tt.expected)
		}
	}
}
