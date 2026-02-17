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
		msg      TimestampedMessage
		wantUser bool // true if should contain "You:", false for "AI:"
	}{
		{
			name: "user message",
			msg: TimestampedMessage{
				Message: types.Message{
					Text:  "hello world",
					IsBot: false,
				},
				Timestamp: time.Now(),
			},
			wantUser: true,
		},
		{
			name: "AI message",
			msg: TimestampedMessage{
				Message: types.Message{
					Text:  "Hi! How can I help?",
					IsBot: true,
				},
				Timestamp: time.Now(),
			},
			wantUser: false,
		},
		{
			name: "multi-line user message",
			msg: TimestampedMessage{
				Message: types.Message{
					Text:  "line1\nline2\nline3",
					IsBot: false,
				},
				Timestamp: time.Now(),
			},
			wantUser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.wantUser {
				result = formatUserMessage(tt.msg.Text)
			} else {
				result = formatAIMessage(tt.msg.Text)
			}

			if tt.wantUser {
				if !strings.Contains(result, "You:") {
					t.Errorf("expected user message to contain 'You:', got: %s", result)
				}
			} else {
				if !strings.Contains(result, "AI:") {
					t.Errorf("expected AI message to contain 'AI:', got: %s", result)
				}
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

// TestContainsMarkdown tests markdown detection
func TestContainsMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "plain text",
			text:     "Hello, how are you?",
			expected: false,
		},
		{
			name:     "code block",
			text:     "Here's some code:\n```go\nfmt.Println(\"hello\")\n```",
			expected: true,
		},
		{
			name:     "inline code",
			text:     "Use the `fmt` package",
			expected: true,
		},
		{
			name:     "header",
			text:     "# This is a title",
			expected: true,
		},
		{
			name:     "bold text",
			text:     "This is **important**",
			expected: true,
		},
		{
			name:     "bullet list",
			text:     "- Item one\n- Item two",
			expected: true,
		},
		{
			name:     "numbered list",
			text:     "1. First\n2. Second",
			expected: true,
		},
		{
			name:     "blockquote",
			text:     "> This is a quote",
			expected: true,
		},
		{
			name:     "asterisk in middle of word",
			text:     "This*is*fine and not markdown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsMarkdown(tt.text)
			if got != tt.expected {
				t.Errorf("containsMarkdown(%q) = %v, want %v", tt.text, got, tt.expected)
			}
		})
	}
}

// TestRenderMarkdown tests markdown rendering
func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
	}{
		{
			name:  "code block",
			text:  "```go\nfmt.Println(\"hello\")\n```",
			width: 80,
		},
		{
			name:  "header",
			text:  "# Title\n\nSome content",
			width: 80,
		},
		{
			name:  "list",
			text:  "- Item 1\n- Item 2\n- Item 3",
			width: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdown(tt.text, tt.width)
			// Just verify we get some output
			if result == "" {
				t.Error("renderMarkdown returned empty string")
			}
			// Result should be different from input (formatted)
			if result == tt.text {
				t.Error("renderMarkdown should transform the content")
			}
		})
	}
}

// TestRenderIfMarkdown tests conditional markdown rendering
func TestRenderIfMarkdown(t *testing.T) {
	// Plain text should be unchanged
	plainText := "Hello, how are you?"
	result := renderIfMarkdown(plainText, 80)
	if result != plainText {
		t.Errorf("renderIfMarkdown should not change plain text, got %q", result)
	}

	// Markdown text should be rendered
	mdText := "# Title\n\nSome **bold** text"
	result = renderIfMarkdown(mdText, 80)
	if result == mdText {
		t.Error("renderIfMarkdown should render markdown content")
	}
}

// TestAutocompleteFilter tests command autocomplete filtering
func TestAutocompleteFilter(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected int // number of matches
	}{
		{
			name:     "empty prefix returns all",
			prefix:   "",
			expected: 5, // all commands
		},
		{
			name:     "slash returns all",
			prefix:   "/",
			expected: 5,
		},
		{
			name:     "filter /m",
			prefix:   "/m",
			expected: 1, // /model
		},
		{
			name:     "filter /n",
			prefix:   "/n",
			expected: 1, // /new
		},
		{
			name:     "filter /q",
			prefix:   "/q",
			expected: 1, // /quit
		},
		{
			name:     "filter /u",
			prefix:   "/u",
			expected: 1, // /usage
		},
		{
			name:     "filter /h",
			prefix:   "/h",
			expected: 1, // /help
		},
		{
			name:     "no match",
			prefix:   "/xyz",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := filterCommands(tt.prefix)
			if len(matches) != tt.expected {
				t.Errorf("filterCommands(%q) returned %d matches, want %d", tt.prefix, len(matches), tt.expected)
			}
		})
	}
}

// TestAutocompleteNavigation tests autocomplete selection navigation
func TestAutocompleteNavigation(t *testing.T) {
	ac := NewAutocomplete()

	// Update with /m to show /model
	ac.Update("/")

	if !ac.IsActive() {
		t.Error("autocomplete should be active after /")
	}

	// Check we have suggestions
	if len(ac.suggestions) == 0 {
		t.Fatal("expected suggestions")
	}

	// Navigate next
	initialSelected := ac.selected
	ac.Next()
	if ac.selected == initialSelected && len(ac.suggestions) > 1 {
		t.Error("Next() should change selection")
	}

	// Navigate prev
	ac.Prev()
	if ac.selected != initialSelected {
		t.Error("Prev() should return to initial selection")
	}

	// Reset
	ac.Reset()
	if ac.IsActive() {
		t.Error("autocomplete should not be active after Reset()")
	}
}

// TestAutocompleteSelected tests getting selected command
func TestAutocompleteSelected(t *testing.T) {
	ac := NewAutocomplete()
	ac.Update("/m")

	selected := ac.Selected()
	if selected != "/model" {
		t.Errorf("expected /model, got %q", selected)
	}
}

// TestRelativeTime tests relative timestamp formatting
func TestRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "just now",
			duration: 30 * time.Second,
			expected: "just now",
		},
		{
			name:     "1 minute",
			duration: 1 * time.Minute,
			expected: "1m ago",
		},
		{
			name:     "5 minutes",
			duration: 5 * time.Minute,
			expected: "5m ago",
		},
		{
			name:     "1 hour",
			duration: 1 * time.Hour,
			expected: "1h ago",
		},
		{
			name:     "3 hours",
			duration: 3 * time.Hour,
			expected: "3h ago",
		},
		{
			name:     "1 day",
			duration: 24 * time.Hour,
			expected: "1d ago",
		},
		{
			name:     "3 days",
			duration: 72 * time.Hour,
			expected: "3d ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Now().Add(-tt.duration)
			result := relativeTime(ts)
			if result != tt.expected {
				t.Errorf("relativeTime(%v ago) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

// TestRelativeTimeZero tests zero time handling
func TestRelativeTimeZero(t *testing.T) {
	result := relativeTime(time.Time{})
	if result != "" {
		t.Errorf("relativeTime(zero) = %q, want empty string", result)
	}
}

// TestFormatProgressBar tests progress bar rendering
func TestFormatProgressBar(t *testing.T) {
	tests := []struct {
		name  string
		used  int
		total int
		width int
	}{
		{
			name:  "empty",
			used:  0,
			total: 80000,
			width: 60,
		},
		{
			name:  "half full",
			used:  40000,
			total: 80000,
			width: 60,
		},
		{
			name:  "full",
			used:  80000,
			total: 80000,
			width: 60,
		},
		{
			name:  "over limit",
			used:  100000,
			total: 80000,
			width: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProgressBar(tt.used, tt.total, tt.width)
			if !strings.Contains(result, "Context:") {
				t.Error("progress bar should contain 'Context:'")
			}
			if !strings.Contains(result, "%") {
				t.Error("progress bar should contain percentage")
			}
		})
	}
}

// TestFormatToolStatus tests tool status formatting
func TestFormatToolStatus(t *testing.T) {
	result := formatToolStatus("web_search", "query=golang")
	if !strings.Contains(result, "🔧") {
		t.Error("tool status should contain tool emoji")
	}
	if !strings.Contains(result, "web_search") {
		t.Error("tool status should contain tool name")
	}
	if !strings.Contains(result, "query=golang") {
		t.Error("tool status should contain args")
	}

	// Test without args
	result = formatToolStatus("calculator", "")
	if !strings.Contains(result, "calculator") {
		t.Error("tool status should contain tool name")
	}
}

// TestFormatStreaming tests streaming message formatting
func TestFormatStreaming(t *testing.T) {
	// Empty text
	result := formatStreaming("")
	if !strings.Contains(result, "AI:") {
		t.Error("streaming should contain AI prefix")
	}
	if !strings.Contains(result, "▋") {
		t.Error("streaming should contain cursor")
	}

	// With text
	result = formatStreaming("Hello world")
	if !strings.Contains(result, "Hello world") {
		t.Error("streaming should contain text")
	}
	if !strings.Contains(result, "▋") {
		t.Error("streaming should contain cursor")
	}
}

// TestRenderHeader tests header rendering
func TestRenderHeader(t *testing.T) {
	result := renderHeader("claude-sonnet-4", 150*time.Millisecond, 80)
	if !strings.Contains(result, "FeelPulse") {
		t.Error("header should contain app name")
	}
	if !strings.Contains(result, "claude-sonnet") {
		t.Error("header should contain model name")
	}
	if !strings.Contains(result, "ms") {
		t.Error("header should contain response time")
	}
}

// TestFormatKeyboardShortcuts tests keyboard shortcuts help
func TestFormatKeyboardShortcuts(t *testing.T) {
	result := formatKeyboardShortcuts()
	shortcuts := []string{"Enter", "Ctrl+L", "Ctrl+R", "Ctrl+C"}
	for _, shortcut := range shortcuts {
		if !strings.Contains(result, shortcut) {
			t.Errorf("keyboard shortcuts should contain %q", shortcut)
		}
	}
}
