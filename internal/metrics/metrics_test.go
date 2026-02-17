package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestCollectorMessagesTotal(t *testing.T) {
	c := NewCollector()

	c.IncrementMessages("telegram")
	c.IncrementMessages("telegram")
	c.IncrementMessages("discord")

	messages := c.GetMessagesTotal()

	if messages["telegram"] != 2 {
		t.Errorf("Expected telegram=2, got %d", messages["telegram"])
	}
	if messages["discord"] != 1 {
		t.Errorf("Expected discord=1, got %d", messages["discord"])
	}
}

func TestCollectorTokens(t *testing.T) {
	c := NewCollector()

	c.AddTokens(100, 50)
	c.AddTokens(200, 100)

	input, output := c.GetTokensTotal()

	if input != 300 {
		t.Errorf("Expected input=300, got %d", input)
	}
	if output != 150 {
		t.Errorf("Expected output=150, got %d", output)
	}
}

func TestCollectorActiveSessions(t *testing.T) {
	c := NewCollector()

	c.SetActiveSessions(5)
	if c.GetActiveSessions() != 5 {
		t.Errorf("Expected active sessions=5, got %d", c.GetActiveSessions())
	}

	c.SetActiveSessions(10)
	if c.GetActiveSessions() != 10 {
		t.Errorf("Expected active sessions=10, got %d", c.GetActiveSessions())
	}
}

func TestCollectorToolCalls(t *testing.T) {
	c := NewCollector()

	c.IncrementToolCall("web_search")
	c.IncrementToolCall("web_search")
	c.IncrementToolCall("exec")
	c.IncrementToolError("exec")

	calls := c.GetToolCalls()
	errors := c.GetToolErrors()

	if calls["web_search"] != 2 {
		t.Errorf("Expected web_search=2, got %d", calls["web_search"])
	}
	if calls["exec"] != 1 {
		t.Errorf("Expected exec=1, got %d", calls["exec"])
	}
	if errors["exec"] != 1 {
		t.Errorf("Expected exec errors=1, got %d", errors["exec"])
	}
}

func TestPrometheusFormat(t *testing.T) {
	c := NewCollector()

	c.IncrementMessages("telegram")
	c.IncrementMessages("telegram")
	c.AddTokens(100, 50)
	c.SetActiveSessions(3)
	c.IncrementToolCall("web_search")

	buf := &bytes.Buffer{}
	c.WritePrometheus(buf)
	output := buf.String()

	// Check for expected metrics
	expectedLines := []string{
		"# HELP feelpulse_messages_total Total messages processed",
		"# TYPE feelpulse_messages_total counter",
		`feelpulse_messages_total{channel="telegram"} 2`,
		"# HELP feelpulse_tokens_total Total tokens used",
		"# TYPE feelpulse_tokens_total counter",
		`feelpulse_tokens_total{type="input"} 100`,
		`feelpulse_tokens_total{type="output"} 50`,
		"# HELP feelpulse_active_sessions Current active sessions",
		"# TYPE feelpulse_active_sessions gauge",
		"feelpulse_active_sessions 3",
		"# HELP feelpulse_tool_calls_total Tool calls by tool name",
		"# TYPE feelpulse_tool_calls_total counter",
		`feelpulse_tool_calls_total{tool="web_search"} 1`,
	}

	for _, line := range expectedLines {
		if !strings.Contains(output, line) {
			t.Errorf("Missing expected line: %s\nGot:\n%s", line, output)
		}
	}
}

func TestCollectorConcurrency(t *testing.T) {
	c := NewCollector()

	// Run concurrent increments
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.IncrementMessages("telegram")
				c.IncrementToolCall("exec")
				c.AddTokens(1, 1)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	messages := c.GetMessagesTotal()
	calls := c.GetToolCalls()
	input, output := c.GetTokensTotal()

	if messages["telegram"] != 1000 {
		t.Errorf("Expected telegram=1000, got %d", messages["telegram"])
	}
	if calls["exec"] != 1000 {
		t.Errorf("Expected exec=1000, got %d", calls["exec"])
	}
	if input != 1000 || output != 1000 {
		t.Errorf("Expected tokens 1000/1000, got %d/%d", input, output)
	}
}

func TestDefaultCollector(t *testing.T) {
	// Reset default collector for clean test
	defaultCollector = NewCollector()

	IncrementMessages("api")
	AddTokens(10, 5)
	SetActiveSessions(1)
	IncrementToolCall("browse")
	IncrementToolError("browse")

	c := Default()
	messages := c.GetMessagesTotal()
	input, output := c.GetTokensTotal()
	calls := c.GetToolCalls()
	errors := c.GetToolErrors()

	if messages["api"] != 1 {
		t.Error("Default collector IncrementMessages failed")
	}
	if input != 10 || output != 5 {
		t.Error("Default collector AddTokens failed")
	}
	if c.GetActiveSessions() != 1 {
		t.Error("Default collector SetActiveSessions failed")
	}
	if calls["browse"] != 1 {
		t.Error("Default collector IncrementToolCall failed")
	}
	if errors["browse"] != 1 {
		t.Error("Default collector IncrementToolError failed")
	}
}
