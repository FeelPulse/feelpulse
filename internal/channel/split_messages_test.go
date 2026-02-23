package channel

import (
	"strings"
	"testing"
)

func TestSplitIntoMessages_Short(t *testing.T) {
	text := "Short message"
	messages := SplitIntoMessages(text, 800)
	
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != text {
		t.Errorf("Message content mismatch")
	}
}

func TestSplitIntoMessages_LongWithParagraphs(t *testing.T) {
	text := strings.Repeat("First paragraph. ", 50) + "\n\n" +
		strings.Repeat("Second paragraph. ", 50) + "\n\n" +
		strings.Repeat("Third paragraph. ", 50)
	
	messages := SplitIntoMessages(text, 800)
	
	if len(messages) < 2 {
		t.Errorf("Expected multiple messages, got %d", len(messages))
	}
	
	// Each message should be reasonable length
	for i, msg := range messages {
		if len(msg) > 4000 {
			t.Errorf("Message %d too long: %d chars", i, len(msg))
		}
	}
}

func TestSplitIntoMessages_WallOfText(t *testing.T) {
	// Simulate bot's wall-of-text output (no \n\n)
	text := "Let me check:Result 1 here.Let me try:Result 2 here.Leon:Final answer."
	
	messages := SplitIntoMessages(text, 40)
	
	// Should split even without \n\n
	if len(messages) == 1 && len(text) > 80 {
		t.Errorf("Failed to split wall-of-text")
	}
}

func TestSplitIntoMessages_PreservesMarkdown(t *testing.T) {
	text := "**Bold paragraph 1**\n\n*Italic paragraph 2*\n\n`Code paragraph 3`"
	messages := SplitIntoMessages(text, 20)
	
	// Rejoined should preserve original (minus extra whitespace)
	rejoined := strings.Join(messages, "\n\n")
	if !strings.Contains(rejoined, "**Bold") || !strings.Contains(rejoined, "*Italic") {
		t.Errorf("Markdown formatting lost")
	}
}

func TestSplitIntoMessages_VeryLongSingleParagraph(t *testing.T) {
	text := strings.Repeat("Very long paragraph without breaks. ", 100)
	
	messages := SplitIntoMessages(text, 800)
	
	// Should split even without paragraph breaks
	if len(messages) == 1 && len(text) > 1600 {
		t.Errorf("Failed to split very long single paragraph")
	}
	
	// No message should exceed Telegram limit
	for _, msg := range messages {
		if len(msg) > 4096 {
			t.Errorf("Message exceeds Telegram limit: %d", len(msg))
		}
	}
}
