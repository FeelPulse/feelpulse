package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

// Collector holds all metrics
type Collector struct {
	messagesTotal  map[string]*atomic.Int64 // by channel
	tokensInput    atomic.Int64
	tokensOutput   atomic.Int64
	activeSessions atomic.Int64
	toolCalls      map[string]*atomic.Int64 // by tool name
	toolErrors     map[string]*atomic.Int64 // by tool name
	mu             sync.RWMutex
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		messagesTotal: make(map[string]*atomic.Int64),
		toolCalls:     make(map[string]*atomic.Int64),
		toolErrors:    make(map[string]*atomic.Int64),
	}
}

// IncrementMessages increments the message counter for a channel
func (c *Collector) IncrementMessages(channel string) {
	c.mu.Lock()
	counter, ok := c.messagesTotal[channel]
	if !ok {
		counter = &atomic.Int64{}
		c.messagesTotal[channel] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// AddTokens adds token usage
func (c *Collector) AddTokens(input, output int) {
	c.tokensInput.Add(int64(input))
	c.tokensOutput.Add(int64(output))
}

// SetActiveSessions sets the number of active sessions
func (c *Collector) SetActiveSessions(count int) {
	c.activeSessions.Store(int64(count))
}

// IncrementToolCall increments the tool call counter
func (c *Collector) IncrementToolCall(toolName string) {
	c.mu.Lock()
	counter, ok := c.toolCalls[toolName]
	if !ok {
		counter = &atomic.Int64{}
		c.toolCalls[toolName] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// IncrementToolError increments the tool error counter
func (c *Collector) IncrementToolError(toolName string) {
	c.mu.Lock()
	counter, ok := c.toolErrors[toolName]
	if !ok {
		counter = &atomic.Int64{}
		c.toolErrors[toolName] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// GetMessagesTotal returns messages total by channel
func (c *Collector) GetMessagesTotal() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]int64)
	for ch, counter := range c.messagesTotal {
		result[ch] = counter.Load()
	}
	return result
}

// GetTokensTotal returns token counts
func (c *Collector) GetTokensTotal() (input, output int64) {
	return c.tokensInput.Load(), c.tokensOutput.Load()
}

// GetActiveSessions returns the number of active sessions
func (c *Collector) GetActiveSessions() int64 {
	return c.activeSessions.Load()
}

// GetToolCalls returns tool call counts
func (c *Collector) GetToolCalls() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]int64)
	for name, counter := range c.toolCalls {
		result[name] = counter.Load()
	}
	return result
}

// GetToolErrors returns tool error counts
func (c *Collector) GetToolErrors() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]int64)
	for name, counter := range c.toolErrors {
		result[name] = counter.Load()
	}
	return result
}

// WritePrometheus writes metrics in Prometheus text format
func (c *Collector) WritePrometheus(w io.Writer) {
	// Messages total
	fmt.Fprintln(w, "# HELP feelpulse_messages_total Total messages processed")
	fmt.Fprintln(w, "# TYPE feelpulse_messages_total counter")
	messages := c.GetMessagesTotal()
	channels := sortedKeys(messages)
	for _, ch := range channels {
		fmt.Fprintf(w, "feelpulse_messages_total{channel=%q} %d\n", ch, messages[ch])
	}

	fmt.Fprintln(w)

	// Tokens total
	input, output := c.GetTokensTotal()
	fmt.Fprintln(w, "# HELP feelpulse_tokens_total Total tokens used")
	fmt.Fprintln(w, "# TYPE feelpulse_tokens_total counter")
	fmt.Fprintf(w, "feelpulse_tokens_total{type=\"input\"} %d\n", input)
	fmt.Fprintf(w, "feelpulse_tokens_total{type=\"output\"} %d\n", output)

	fmt.Fprintln(w)

	// Active sessions
	fmt.Fprintln(w, "# HELP feelpulse_active_sessions Current active sessions")
	fmt.Fprintln(w, "# TYPE feelpulse_active_sessions gauge")
	fmt.Fprintf(w, "feelpulse_active_sessions %d\n", c.GetActiveSessions())

	fmt.Fprintln(w)

	// Tool calls
	fmt.Fprintln(w, "# HELP feelpulse_tool_calls_total Tool calls by tool name")
	fmt.Fprintln(w, "# TYPE feelpulse_tool_calls_total counter")
	toolCalls := c.GetToolCalls()
	toolNames := sortedKeys(toolCalls)
	for _, name := range toolNames {
		fmt.Fprintf(w, "feelpulse_tool_calls_total{tool=%q} %d\n", name, toolCalls[name])
	}

	fmt.Fprintln(w)

	// Tool errors
	fmt.Fprintln(w, "# HELP feelpulse_tool_errors_total Tool errors by tool name")
	fmt.Fprintln(w, "# TYPE feelpulse_tool_errors_total counter")
	toolErrors := c.GetToolErrors()
	errorNames := sortedKeys(toolErrors)
	for _, name := range errorNames {
		fmt.Fprintf(w, "feelpulse_tool_errors_total{tool=%q} %d\n", name, toolErrors[name])
	}
}

// sortedKeys returns sorted keys of a map
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Handler returns an HTTP handler for the metrics endpoint
func (c *Collector) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		c.WritePrometheus(w)
	}
}

// Global collector instance
var defaultCollector = NewCollector()

// Default returns the default metrics collector
func Default() *Collector {
	return defaultCollector
}

// IncrementMessages increments messages on the default collector
func IncrementMessages(channel string) {
	defaultCollector.IncrementMessages(channel)
}

// AddTokens adds tokens on the default collector
func AddTokens(input, output int) {
	defaultCollector.AddTokens(input, output)
}

// SetActiveSessions sets active sessions on the default collector
func SetActiveSessions(count int) {
	defaultCollector.SetActiveSessions(count)
}

// IncrementToolCall increments tool calls on the default collector
func IncrementToolCall(toolName string) {
	defaultCollector.IncrementToolCall(toolName)
}

// IncrementToolError increments tool errors on the default collector
func IncrementToolError(toolName string) {
	defaultCollector.IncrementToolError(toolName)
}

// Handler returns the default collector's HTTP handler
func Handler() http.HandlerFunc {
	return defaultCollector.Handler()
}
