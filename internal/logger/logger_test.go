package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"", INFO}, // Default
		{"warn", WARN},
		{"WARN", WARN},
		{"warning", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"invalid", INFO}, // Default for unknown
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLevel(tt.input)
			if got != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.expected {
				t.Errorf("Level(%d).String() = %v, want %v", tt.level, got, tt.expected)
			}
		})
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "warn", Component: "test"})
	l.SetOutput(buf)

	// These should be filtered out
	l.Debug("debug message")
	l.Info("info message")

	// These should appear
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()

	if strings.Contains(output, "DEBUG") {
		t.Error("DEBUG message should have been filtered")
	}
	if strings.Contains(output, "INFO") {
		t.Error("INFO message should have been filtered")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("WARN message should have been logged")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("ERROR message should have been logged")
	}
}

func TestLoggerFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "info", Component: "gateway"})
	l.SetOutput(buf)

	l.Info("Processing message from %s", "testuser")

	output := buf.String()

	// Check format: "2006-01-02 15:04:05 INFO [gateway] Processing message from testuser"
	if !strings.Contains(output, "INFO") {
		t.Error("Output should contain level")
	}
	if !strings.Contains(output, "[gateway]") {
		t.Error("Output should contain component")
	}
	if !strings.Contains(output, "Processing message from testuser") {
		t.Error("Output should contain formatted message")
	}
}

func TestContextLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "debug", Component: "agent"})
	l.SetOutput(buf)

	ctx := l.WithRequestID("req-12345")
	ctx.Info("Handling request")

	output := buf.String()

	// Check format includes request ID
	if !strings.Contains(output, "[req-12345]") {
		t.Errorf("Output should contain request ID, got: %s", output)
	}
	if !strings.Contains(output, "[agent]") {
		t.Error("Output should contain component")
	}
}

func TestWithComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "info", Component: "main"})
	l.SetOutput(buf)

	child := l.WithComponent("gateway")
	child.Info("Gateway message")

	output := buf.String()

	if !strings.Contains(output, "[gateway]") {
		t.Errorf("Output should contain child component, got: %s", output)
	}
}

func TestSetLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "error", Component: "test"})
	l.SetOutput(buf)

	// Initially only ERROR should log
	l.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("INFO should be filtered at ERROR level")
	}

	// Change level to INFO
	l.SetLevel(INFO)
	l.Info("should appear")

	if !strings.Contains(buf.String(), "should appear") {
		t.Error("INFO should log after level change")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(&Config{Level: "debug", Component: "pkg"})
	l.SetOutput(buf)
	SetDefaultLogger(l)

	Debug("debug from pkg")
	Info("info from pkg")
	Warn("warn from pkg")
	Error("error from pkg")

	output := buf.String()

	if !strings.Contains(output, "DEBUG") {
		t.Error("Package Debug() should work")
	}
	if !strings.Contains(output, "INFO") {
		t.Error("Package Info() should work")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("Package Warn() should work")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("Package Error() should work")
	}
}
