package session

import (
	"strings"
	"testing"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"hi", 0},          // 2/4 = 0
		{"hello", 1},       // 5/4 = 1
		{"hello world", 2}, // 11/4 = 2
		{strings.Repeat("a", 100), 25},
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.text)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.text, got, tt.expected)
		}
	}
}

func TestEstimateHistoryTokens(t *testing.T) {
	messages := []types.Message{
		{Text: strings.Repeat("a", 100)}, // 25 tokens
		{Text: strings.Repeat("b", 200)}, // 50 tokens
		{Text: strings.Repeat("c", 300)}, // 75 tokens
	}

	got := EstimateHistoryTokens(messages)
	want := 150 // 25 + 50 + 75

	if got != want {
		t.Errorf("EstimateHistoryTokens = %d, want %d", got, want)
	}
}

func TestNeedsCompaction(t *testing.T) {
	smallHistory := []types.Message{
		{Text: strings.Repeat("a", 100)}, // 25 tokens
	}

	largeHistory := make([]types.Message, 0)
	for i := 0; i < 100; i++ {
		largeHistory = append(largeHistory, types.Message{
			Text: strings.Repeat("x", 4000), // 1000 tokens each
		})
	}
	// 100 * 1000 = 100,000 tokens

	if NeedsCompaction(smallHistory, 80000) {
		t.Error("Small history should not need compaction")
	}

	if !NeedsCompaction(largeHistory, 80000) {
		t.Error("Large history should need compaction")
	}
}

func TestCompactor_SplitMessages(t *testing.T) {
	// Create 10 messages, each 1000 tokens
	messages := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = types.Message{
			Text: strings.Repeat("a", 4000), // 1000 tokens each
		}
	}
	// Total: 10,000 tokens

	// Keep recent 5000 tokens (should be last 5 messages)
	c := NewCompactor(nil, 10000, 5000)
	toSummarize, toKeep := c.SplitMessages(messages)

	if len(toKeep) != 5 {
		t.Errorf("toKeep len = %d, want 5", len(toKeep))
	}
	if len(toSummarize) != 5 {
		t.Errorf("toSummarize len = %d, want 5", len(toSummarize))
	}

	// Verify the last 5 messages are kept (most recent)
	for i := 0; i < 5; i++ {
		if toKeep[i].Text != messages[5+i].Text {
			t.Error("toKeep should contain the most recent messages")
		}
	}
}

func TestCompactor_SplitMessages_KeepAll(t *testing.T) {
	// Only 3 small messages (total ~1 token)
	messages := []types.Message{
		{Text: "one"},   // 0 tokens (3/4=0)
		{Text: "two"},   // 0 tokens
		{Text: "three"}, // 1 token (5/4=1)
	}

	// keepRecentTokens = 500, which is way more than we have
	c := NewCompactor(nil, 1000, 500)
	toSummarize, toKeep := c.SplitMessages(messages)

	// Should keep all since total tokens < keepRecentTokens
	if len(toKeep) != 3 {
		t.Errorf("toKeep len = %d, want 3", len(toKeep))
	}
	if len(toSummarize) != 0 {
		t.Errorf("toSummarize should be empty, got %d", len(toSummarize))
	}
}

func TestCompactor_CreateSummaryMessage(t *testing.T) {
	summaryText := "This is a conversation summary."
	msg := CreateSummaryMessage(summaryText)

	if msg.Text != summaryText {
		t.Errorf("Summary text = %q, want %q", msg.Text, summaryText)
	}
	if !msg.IsBot {
		t.Error("Summary message should be from bot")
	}
	if msg.Metadata["type"] != "summary" {
		t.Error("Summary message should have type=summary metadata")
	}
}

// MockSummarizer for testing compaction without real API calls
type MockSummarizer struct {
	called  bool
	summary string
	err     error
}

func (m *MockSummarizer) Summarize(messages []types.Message) (string, error) {
	m.called = true
	return m.summary, m.err
}

func TestCompactor_CompactIfNeeded_BelowThreshold(t *testing.T) {
	messages := []types.Message{
		{Text: "hello"},
		{Text: "world"},
	}

	mock := &MockSummarizer{summary: "ignored"}
	c := NewCompactor(mock, 80000, 10000)

	result, err := c.CompactIfNeeded(messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded error: %v", err)
	}

	// Should return original messages unchanged
	if len(result) != 2 {
		t.Errorf("Expected original messages, got %d", len(result))
	}
	if mock.called {
		t.Error("Summarizer should not be called below threshold")
	}
}

func TestCompactor_CompactIfNeeded_AboveThreshold(t *testing.T) {
	// Create messages exceeding threshold
	messages := make([]types.Message, 0)
	for i := 0; i < 100; i++ {
		messages = append(messages, types.Message{
			Text: strings.Repeat("x", 4000), // 1000 tokens each
		})
	}
	// Total: 100,000 tokens, above 80k threshold

	mock := &MockSummarizer{summary: "Summary of earlier conversation."}
	c := NewCompactor(mock, 80000, 10000) // keep recent 10k tokens = last 10 messages

	result, err := c.CompactIfNeeded(messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded error: %v", err)
	}

	if !mock.called {
		t.Error("Summarizer should be called when above threshold")
	}

	// Should have: 1 summary + 10 kept = 11 messages
	if len(result) != 11 {
		t.Errorf("Expected 11 messages (1 summary + 10 kept), got %d", len(result))
	}

	// First message should be the summary
	if result[0].Metadata["type"] != "summary" {
		t.Error("First message should be the summary")
	}
	if result[0].Text != mock.summary {
		t.Errorf("Summary text = %q, want %q", result[0].Text, mock.summary)
	}
}

func TestCompactor_CompactIfNeeded_PreserveRecentMessages(t *testing.T) {
	// Create messages exceeding threshold
	messages := make([]types.Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = types.Message{
			Text: strings.Repeat("x", 20000), // 5000 tokens each
			From: "user" + string(rune('a'+i)),
		}
	}
	// Total: 100,000 tokens

	mock := &MockSummarizer{summary: "Summary"}
	c := NewCompactor(mock, 80000, 25000) // keep recent 25k tokens = last 5 messages

	result, err := c.CompactIfNeeded(messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded error: %v", err)
	}

	// Verify last 5 messages are preserved
	for i := 0; i < 5; i++ {
		expected := messages[15+i]
		actual := result[1+i] // skip summary at index 0
		if actual.From != expected.From {
			t.Errorf("Message %d: From = %q, want %q", i, actual.From, expected.From)
		}
	}
}
