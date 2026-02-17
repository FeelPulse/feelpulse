package usage

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Stats holds usage statistics for a session
type Stats struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	RequestCount int
	ModelsUsed   map[string]int
	FirstRequest time.Time
	LastRequest  time.Time
}

// String returns a human-readable summary of usage
func (s *Stats) String() string {
	if s.RequestCount == 0 {
		return "📊 No usage recorded yet."
	}

	var sb strings.Builder
	sb.WriteString("📊 *Usage Statistics*\n\n")
	sb.WriteString(fmt.Sprintf("🔢 Total Tokens: %d\n", s.TotalTokens))
	sb.WriteString(fmt.Sprintf("   ↳ Input: %d\n", s.InputTokens))
	sb.WriteString(fmt.Sprintf("   ↳ Output: %d\n", s.OutputTokens))
	sb.WriteString(fmt.Sprintf("💬 Requests: %d\n", s.RequestCount))

	if len(s.ModelsUsed) > 0 {
		sb.WriteString("\n🤖 Models Used:\n")
		for model, count := range s.ModelsUsed {
			sb.WriteString(fmt.Sprintf("   • %s: %d requests\n", model, count))
		}
	}

	if !s.FirstRequest.IsZero() {
		duration := time.Since(s.FirstRequest)
		sb.WriteString(fmt.Sprintf("\n⏱️ Session duration: %s\n", formatDuration(duration)))
	}

	return sb.String()
}

// Tracker manages usage statistics per session
type Tracker struct {
	stats map[string]*Stats
	mu    sync.RWMutex
}

// NewTracker creates a new usage tracker
func NewTracker() *Tracker {
	return &Tracker{
		stats: make(map[string]*Stats),
	}
}

// sessionKey generates a unique key for channel+user
func sessionKey(channel, userID string) string {
	return channel + ":" + userID
}

// Record records token usage for a session
func (t *Tracker) Record(channel, userID string, inputTokens, outputTokens int, model string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := sessionKey(channel, userID)
	stats, exists := t.stats[key]
	if !exists {
		stats = &Stats{
			ModelsUsed:   make(map[string]int),
			FirstRequest: time.Now(),
		}
		t.stats[key] = stats
	}

	stats.InputTokens += inputTokens
	stats.OutputTokens += outputTokens
	stats.TotalTokens += inputTokens + outputTokens
	stats.RequestCount++
	stats.LastRequest = time.Now()

	if model != "" {
		stats.ModelsUsed[model]++
	}
}

// Get retrieves usage stats for a session
func (t *Tracker) Get(channel, userID string) *Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := sessionKey(channel, userID)
	stats, exists := t.stats[key]
	if !exists {
		return &Stats{
			ModelsUsed: make(map[string]int),
		}
	}

	// Return a copy to avoid race conditions
	copy := &Stats{
		InputTokens:  stats.InputTokens,
		OutputTokens: stats.OutputTokens,
		TotalTokens:  stats.TotalTokens,
		RequestCount: stats.RequestCount,
		FirstRequest: stats.FirstRequest,
		LastRequest:  stats.LastRequest,
		ModelsUsed:   make(map[string]int),
	}
	for k, v := range stats.ModelsUsed {
		copy.ModelsUsed[k] = v
	}
	return copy
}

// Reset clears usage stats for a session
func (t *Tracker) Reset(channel, userID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := sessionKey(channel, userID)
	delete(t.stats, key)
}

// GetGlobal returns aggregated stats across all sessions
func (t *Tracker) GetGlobal() *Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	global := &Stats{
		ModelsUsed: make(map[string]int),
	}

	for _, stats := range t.stats {
		global.InputTokens += stats.InputTokens
		global.OutputTokens += stats.OutputTokens
		global.TotalTokens += stats.TotalTokens
		global.RequestCount += stats.RequestCount

		if global.FirstRequest.IsZero() || (!stats.FirstRequest.IsZero() && stats.FirstRequest.Before(global.FirstRequest)) {
			global.FirstRequest = stats.FirstRequest
		}
		if stats.LastRequest.After(global.LastRequest) {
			global.LastRequest = stats.LastRequest
		}

		for model, count := range stats.ModelsUsed {
			global.ModelsUsed[model] += count
		}
	}

	return global
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dh", hours)
}
