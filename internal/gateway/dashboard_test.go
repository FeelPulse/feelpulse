package gateway

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDashboardData_Marshal(t *testing.T) {
	data := DashboardData{
		Status:         "running",
		Version:        "0.1.0",
		Uptime:         "2h 30m",
		UptimeSeconds:  9000,
		StartedAt:      time.Now().Add(-2*time.Hour - 30*time.Minute).Format(time.RFC3339),
		ActiveSessions: 5,
		TotalTokens:    12500,
		InputTokens:    8000,
		OutputTokens:   4500,
		TotalRequests:  42,
		Channels: map[string]bool{
			"telegram": true,
			"discord":  false,
		},
		Agent: "anthropic/claude-sonnet-4-20250514",
		RecentActivity: []ActivityEntry{
			{
				Timestamp: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
				Channel:   "telegram",
				User:      "john",
				Preview:   "Hello, how are you?",
			},
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal dashboard data: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Failed to parse marshaled data: %v", err)
	}

	if parsed["status"] != "running" {
		t.Errorf("Expected status 'running', got '%v'", parsed["status"])
	}

	if parsed["active_sessions"].(float64) != 5 {
		t.Errorf("Expected active_sessions 5, got %v", parsed["active_sessions"])
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		seconds  int64
		expected string
	}{
		{30, "30s"},
		{90, "1m 30s"},
		{3600, "1h 0m"},
		{3661, "1h 1m"},
		{86400, "1d 0h"},
		{90061, "1d 1h"},
	}

	for _, tt := range tests {
		result := formatUptime(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatUptime(%d) = %s, want %s", tt.seconds, result, tt.expected)
		}
	}
}

func TestTruncatePreview(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc..."},
	}

	for _, tt := range tests {
		result := truncatePreview(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncatePreview(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestGenerateDashboardHTML(t *testing.T) {
	data := DashboardData{
		Status:         "running",
		Version:        "0.1.0",
		Uptime:         "1h 30m",
		ActiveSessions: 3,
		TotalTokens:    5000,
		Agent:          "claude-sonnet-4",
		Channels: map[string]bool{
			"telegram": true,
		},
	}

	html := generateDashboardHTML(data)

	// Check for key elements
	if html == "" {
		t.Fatal("Expected non-empty HTML")
	}

	if !containsStr(html, "FeelPulse Dashboard") {
		t.Error("HTML should contain title")
	}

	if !containsStr(html, "running") {
		t.Error("HTML should contain status")
	}

	if !containsStr(html, "1h 30m") {
		t.Error("HTML should contain uptime")
	}

	if !containsStr(html, "5000") {
		t.Error("HTML should contain token count")
	}

	if !containsStr(html, "telegram") {
		t.Error("HTML should contain channel name")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
