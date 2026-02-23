package session

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// BenchmarkSessionGet benchmarks session lookup speed
func BenchmarkSessionGet(b *testing.B) {
	store := NewStore()

	// Pre-populate with sessions
	for i := 0; i < 100; i++ {
		sess := store.GetOrCreate("telegram", fmt.Sprintf("user%d", i))
		sess.AddMessage(types.Message{Text: "Hello", IsBot: false})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		userID := fmt.Sprintf("user%d", i%100)
		store.GetOrCreate("telegram", userID)
	}
}

// BenchmarkSessionGetOrCreate benchmarks session creation
func BenchmarkSessionGetOrCreate(b *testing.B) {
	store := NewStore()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		userID := fmt.Sprintf("user%d", i)
		store.GetOrCreate("telegram", userID)
	}
}

// BenchmarkSessionAddMessage benchmarks adding messages to sessions
func BenchmarkSessionAddMessage(b *testing.B) {
	store := NewStore()
	sess := store.GetOrCreate("telegram", "user1")

	msg := types.Message{
		Text:      "Hello, this is a test message!",
		From:      "user",
		Channel:   "telegram",
		IsBot:     false,
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sess.AddMessage(msg)
	}
}

// BenchmarkSessionGetAllMessages benchmarks retrieving all messages
func BenchmarkSessionGetAllMessages(b *testing.B) {
	store := NewStore()
	sess := store.GetOrCreate("telegram", "user1")

	// Add 50 messages
	for i := 0; i < 50; i++ {
		sess.AddMessage(types.Message{
			Text:  fmt.Sprintf("Message %d", i),
			IsBot: i%2 == 1,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sess.GetAllMessages()
	}
}

// BenchmarkSessionGetHistory benchmarks retrieving last N messages
func BenchmarkSessionGetHistory(b *testing.B) {
	store := NewStore()
	sess := store.GetOrCreate("telegram", "user1")

	// Add 100 messages
	for i := 0; i < 100; i++ {
		sess.AddMessage(types.Message{
			Text:  fmt.Sprintf("Message %d", i),
			IsBot: i%2 == 1,
		})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sess.GetHistory(10)
	}
}

// BenchmarkContextCompaction benchmarks the compaction operation
func BenchmarkContextCompaction(b *testing.B) {
	// Create mock summarizer
	mockSummarizer := &BenchmarkSummarizer{}

	// Create compactor with realistic threshold
	compactor := NewCompactor(mockSummarizer, 80000, 15000)

	// Create large message history
	messages := make([]types.Message, 100)
	for i := 0; i < 100; i++ {
		messages[i] = types.Message{
			Text:  strings.Repeat("x", 4000), // 1000 tokens each = 100k total
			IsBot: i%2 == 1,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = compactor.CompactIfNeeded(messages)
	}
}

// BenchmarkSummarizer is a fast mock summarizer for benchmarks
type BenchmarkSummarizer struct{}

func (s *BenchmarkSummarizer) Summarize(messages []types.Message) (string, error) {
	return "[Summary of conversation]", nil
}

// BenchmarkEstimateTokens benchmarks token estimation
func BenchmarkEstimateTokens(b *testing.B) {
	text := strings.Repeat("This is a sample text for token estimation. ", 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = EstimateTokens(text)
	}
}

// BenchmarkEstimateHistoryTokens benchmarks history token estimation
func BenchmarkEstimateHistoryTokens(b *testing.B) {
	messages := make([]types.Message, 50)
	for i := 0; i < 50; i++ {
		messages[i] = types.Message{
			Text: strings.Repeat("Sample message text here. ", 10),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = EstimateHistoryTokens(messages)
	}
}

// BenchmarkSessionKey benchmarks session key generation
func BenchmarkSessionKey(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = SessionKey("telegram", "user12345")
	}
}

// BenchmarkNeedsCompaction benchmarks the compaction check
func BenchmarkNeedsCompaction(b *testing.B) {
	messages := make([]types.Message, 50)
	for i := 0; i < 50; i++ {
		messages[i] = types.Message{
			Text: strings.Repeat("x", 1000),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NeedsCompaction(messages, 80000)
	}
}

// BenchmarkSplitMessages benchmarks message splitting
func BenchmarkSplitMessages(b *testing.B) {
	compactor := NewCompactor(nil, 80000, 15000)

	messages := make([]types.Message, 100)
	for i := 0; i < 100; i++ {
		messages[i] = types.Message{
			Text:  fmt.Sprintf("Message %d", i),
			IsBot: i%2 == 1,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = compactor.SplitMessages(messages)
	}
}

// BenchmarkConcurrentSessionAccess benchmarks concurrent session access
func BenchmarkConcurrentSessionAccess(b *testing.B) {
	store := NewStore()

	// Pre-populate
	for i := 0; i < 10; i++ {
		store.GetOrCreate("telegram", fmt.Sprintf("user%d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			userID := fmt.Sprintf("user%d", i%10)
			sess := store.GetOrCreate("telegram", userID)
			sess.AddMessage(types.Message{Text: "test", IsBot: false})
			_ = sess.GetAllMessages()
			i++
		}
	})
}

// BenchmarkSessionClear benchmarks session clearing
func BenchmarkSessionClear(b *testing.B) {
	store := NewStore()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sess := store.GetOrCreate("telegram", fmt.Sprintf("user%d", i))
		for j := 0; j < 10; j++ {
			sess.AddMessage(types.Message{Text: "test"})
		}
		sess.Clear()
	}
}

// BenchmarkSessionCount benchmarks counting sessions
func BenchmarkSessionCount(b *testing.B) {
	store := NewStore()

	// Pre-populate with sessions
	for i := 0; i < 1000; i++ {
		store.GetOrCreate("telegram", fmt.Sprintf("user%d", i))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = store.Count()
	}
}

// BenchmarkGetRecent benchmarks getting recent sessions
func BenchmarkGetRecent(b *testing.B) {
	store := NewStore()

	// Pre-populate with sessions
	for i := 0; i < 100; i++ {
		sess := store.GetOrCreate("telegram", fmt.Sprintf("user%d", i))
		sess.AddMessage(types.Message{Text: "test"})
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = store.GetRecent(10)
	}
}
