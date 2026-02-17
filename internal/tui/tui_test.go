package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// TestFormatMessage tests message formatting for user and AI messages
func TestFormatMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      types.Message
		wantUser bool // true if should contain "You:", false for "AI:"
	}{
		{
			name: "user message",
			msg: types.Message{
				Text:      "hello world",
				IsBot:     false,
				Timestamp: time.Now(),
			},
			wantUser: true,
		},
		{
			name: "AI message",
			msg: types.Message{
				Text:      "Hi! How can I help?",
				IsBot:     true,
				Timestamp: time.Now(),
			},
			wantUser: false,
		},
		{
			name: "multi-line user message",
			msg: types.Message{
				Text:      "line1\nline2\nline3",
				IsBot:     false,
				Timestamp: time.Now(),
			},
			wantUser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMessage(tt.msg)
			if tt.wantUser {
				if !strings.Contains(result, "You:") {
					t.Errorf("expected user message to contain 'You:', got: %s", result)
				}
			} else {
				if !strings.Contains(result, "AI:") {
					t.Errorf("expected AI message to contain 'AI:', got: %s", result)
				}
			}
			if !strings.Contains(result, tt.msg.Text) {
				t.Errorf("expected message to contain text %q, got: %s", tt.msg.Text, result)
			}
		})
	}
}

// TestParseCommand tests slash command parsing
func TestParseCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd Command
		wantArg string
	}{
		{
			input:   "/new",
			wantCmd: CmdNew,
			wantArg: "",
		},
		{
			input:   "/new  ",
			wantCmd: CmdNew,
			wantArg: "",
		},
		{
			input:   "/model",
			wantCmd: CmdModel,
			wantArg: "",
		},
		{
			input:   "/model claude-3-opus-20240229",
			wantCmd: CmdModel,
			wantArg: "claude-3-opus-20240229",
		},
		{
			input:   "/help",
			wantCmd: CmdHelp,
			wantArg: "",
		},
		{
			input:   "/usage",
			wantCmd: CmdUsage,
			wantArg: "",
		},
		{
			input:   "hello world",
			wantCmd: CmdNone,
			wantArg: "",
		},
		{
			input:   "",
			wantCmd: CmdNone,
			wantArg: "",
		},
		{
			input:   "/unknown",
			wantCmd: CmdUnknown,
			wantArg: "",
		},
		{
			input:   "/quit",
			wantCmd: CmdQuit,
			wantArg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, arg := parseCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("parseCommand(%q) cmd = %v, want %v", tt.input, cmd, tt.wantCmd)
			}
			if arg != tt.wantArg {
				t.Errorf("parseCommand(%q) arg = %q, want %q", tt.input, arg, tt.wantArg)
			}
		})
	}
}

// TestIsCommand tests command detection
func TestIsCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/new", true},
		{"/help", true},
		{"/model test", true},
		{"hello", false},
		{"", false},
		{" /new", false}, // leading space means not a command
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isCommand(tt.input)
			if got != tt.want {
				t.Errorf("isCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestHelpText tests that help text contains all commands
func TestHelpText(t *testing.T) {
	help := helpText()
	commands := []string{"/new", "/model", "/usage", "/help", "/quit"}
	for _, cmd := range commands {
		if !strings.Contains(help, cmd) {
			t.Errorf("help text should contain %q", cmd)
		}
	}
}

// TestWrapText tests text wrapping functionality
func TestWrapText(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		width  int
		expect int // expected number of lines
	}{
		{
			name:   "short text no wrap",
			text:   "hello",
			width:  80,
			expect: 1,
		},
		{
			name:   "long text wraps",
			text:   "this is a very long line that should wrap at some point because it exceeds width",
			width:  20,
			expect: 5, // approximate, depends on word boundaries
		},
		{
			name:   "already has newlines",
			text:   "line1\nline2",
			width:  80,
			expect: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.width)
			lines := strings.Split(result, "\n")
			// Just verify we get multiple lines when expected
			if tt.expect > 1 && len(lines) < 2 {
				t.Errorf("wrapText(%q, %d) should produce multiple lines, got %d lines", tt.text, tt.width, len(lines))
			}
		})
	}
}
