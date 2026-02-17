package usage

import (
	"testing"
)

func TestNewTracker(t *testing.T) {
	tracker := NewTracker()
	if tracker == nil {
		t.Fatal("NewTracker returned nil")
	}
}

func TestTrackerRecord(t *testing.T) {
	tracker := NewTracker()

	// Record usage for a session
	tracker.Record("telegram", "user123", 100, 50, "claude-sonnet-4")

	stats := tracker.Get("telegram", "user123")
	if stats == nil {
		t.Fatal("Get returned nil")
	}

	if stats.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", stats.InputTokens)
	}
	if stats.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", stats.OutputTokens)
	}
	if stats.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", stats.TotalTokens)
	}
	if stats.RequestCount != 1 {
		t.Errorf("RequestCount = %d, want 1", stats.RequestCount)
	}
}

func TestTrackerMultipleRecords(t *testing.T) {
	tracker := NewTracker()

	tracker.Record("telegram", "user123", 100, 50, "claude-sonnet-4")
	tracker.Record("telegram", "user123", 200, 100, "claude-sonnet-4")
	tracker.Record("telegram", "user123", 150, 75, "gpt-4o")

	stats := tracker.Get("telegram", "user123")
	if stats.InputTokens != 450 {
		t.Errorf("InputTokens = %d, want 450", stats.InputTokens)
	}
	if stats.OutputTokens != 225 {
		t.Errorf("OutputTokens = %d, want 225", stats.OutputTokens)
	}
	if stats.TotalTokens != 675 {
		t.Errorf("TotalTokens = %d, want 675", stats.TotalTokens)
	}
	if stats.RequestCount != 3 {
		t.Errorf("RequestCount = %d, want 3", stats.RequestCount)
	}
}

func TestTrackerReset(t *testing.T) {
	tracker := NewTracker()

	tracker.Record("telegram", "user123", 100, 50, "claude-sonnet-4")
	tracker.Reset("telegram", "user123")

	stats := tracker.Get("telegram", "user123")
	if stats.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 after reset", stats.TotalTokens)
	}
	if stats.RequestCount != 0 {
		t.Errorf("RequestCount = %d, want 0 after reset", stats.RequestCount)
	}
}

func TestTrackerDifferentUsers(t *testing.T) {
	tracker := NewTracker()

	tracker.Record("telegram", "user1", 100, 50, "claude")
	tracker.Record("telegram", "user2", 200, 100, "claude")

	stats1 := tracker.Get("telegram", "user1")
	stats2 := tracker.Get("telegram", "user2")

	if stats1.TotalTokens != 150 {
		t.Errorf("User1 TotalTokens = %d, want 150", stats1.TotalTokens)
	}
	if stats2.TotalTokens != 300 {
		t.Errorf("User2 TotalTokens = %d, want 300", stats2.TotalTokens)
	}
}

func TestTrackerModelsUsed(t *testing.T) {
	tracker := NewTracker()

	tracker.Record("telegram", "user123", 100, 50, "claude-sonnet-4")
	tracker.Record("telegram", "user123", 200, 100, "gpt-4o")
	tracker.Record("telegram", "user123", 150, 75, "claude-sonnet-4")

	stats := tracker.Get("telegram", "user123")

	if len(stats.ModelsUsed) != 2 {
		t.Errorf("ModelsUsed count = %d, want 2", len(stats.ModelsUsed))
	}

	if stats.ModelsUsed["claude-sonnet-4"] != 2 {
		t.Errorf("claude-sonnet-4 usage = %d, want 2", stats.ModelsUsed["claude-sonnet-4"])
	}
	if stats.ModelsUsed["gpt-4o"] != 1 {
		t.Errorf("gpt-4o usage = %d, want 1", stats.ModelsUsed["gpt-4o"])
	}
}

func TestStatsString(t *testing.T) {
	stats := &Stats{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		RequestCount: 3,
		ModelsUsed: map[string]int{
			"claude-sonnet-4": 2,
			"gpt-4o":          1,
		},
	}

	str := stats.String()
	if str == "" {
		t.Error("Stats.String() should not be empty")
	}

	// Should contain key information
	if !containsSubstring(str, "150") {
		t.Error("Should contain total tokens")
	}
	if !containsSubstring(str, "3") {
		t.Error("Should contain request count")
	}
}

func TestGetNonExistent(t *testing.T) {
	tracker := NewTracker()

	stats := tracker.Get("telegram", "nonexistent")
	if stats == nil {
		t.Fatal("Get should return empty stats, not nil")
	}
	if stats.TotalTokens != 0 {
		t.Error("Non-existent user should have 0 tokens")
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
