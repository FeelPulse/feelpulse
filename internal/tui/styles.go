package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	primaryColor   = lipgloss.Color("#FF6B9D") // Pink/magenta for FeelPulse branding
	secondaryColor = lipgloss.Color("#7C3AED") // Purple accent
	userColor      = lipgloss.Color("#3B82F6") // Blue for user messages
	aiColor        = lipgloss.Color("#10B981") // Green for AI messages
	dimColor       = lipgloss.Color("#6B7280") // Gray for help text
	errorColor     = lipgloss.Color("#EF4444") // Red for errors
	toolColor      = lipgloss.Color("#F59E0B") // Orange for tool calls
	progressFg     = lipgloss.Color("#10B981") // Green for progress bar filled
	progressBg     = lipgloss.Color("#374151") // Dark gray for progress bar empty
)

// Styles
var (
	// Header bar style
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 1)

	// Rich header box style
	headerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1)

	// User message prefix
	userPrefixStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(userColor)

	// AI message prefix
	aiPrefixStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(aiColor)

	// Message text styles
	userTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))

	aiTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3F4F6"))

	// Input area border
	inputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(secondaryColor).
				Padding(0, 1)

	// Help bar at bottom
	helpStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true)

	// Thinking indicator
	thinkingStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true)

	// Error message
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// System message (for commands, status)
	systemStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true)

	// Viewport (chat area) border
	chatBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimColor)

	// Tool call status
	toolStatusStyle = lipgloss.NewStyle().
			Foreground(toolColor).
			Bold(true)

	// Timestamp style
	timestampStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			Italic(true)

	// Progress bar styles
	progressFilledStyle = lipgloss.NewStyle().
				Foreground(progressFg)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(progressBg)

	// Streaming cursor
	streamingCursorStyle = lipgloss.NewStyle().
				Foreground(aiColor).
				Bold(true).
				Blink(true)

	// Keyboard shortcut key style
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	// Keyboard shortcut description style
	keyDescStyle = lipgloss.NewStyle().
			Foreground(dimColor)
)

// formatUserMessage formats a user message with styling
func formatUserMessage(text string) string {
	prefix := userPrefixStyle.Render("You:")
	return prefix + " " + userTextStyle.Render(text)
}

// formatAIMessage formats an AI message with styling
func formatAIMessage(text string) string {
	prefix := aiPrefixStyle.Render("AI:")
	return prefix + " " + aiTextStyle.Render(text)
}

// formatAIMessageWithMarkdown formats an AI message, rendering markdown if present
func formatAIMessageWithMarkdown(text string, width int) string {
	prefix := aiPrefixStyle.Render("AI:")

	// Render markdown if content contains markdown
	rendered := renderIfMarkdown(text, width-6)

	return prefix + " " + rendered
}

// formatSystemMessage formats a system message
func formatSystemMessage(text string) string {
	return systemStyle.Render("• " + text)
}

// formatError formats an error message
func formatError(text string) string {
	return errorStyle.Render("✗ " + text)
}

// formatThinking returns the thinking indicator
func formatThinking() string {
	return thinkingStyle.Render("⏳ Thinking...")
}

// formatStreaming returns the streaming indicator with cursor
func formatStreaming(text string) string {
	prefix := aiPrefixStyle.Render("AI:")
	cursor := streamingCursorStyle.Render("▋")
	if text == "" {
		return prefix + " " + cursor
	}
	return prefix + " " + aiTextStyle.Render(text) + cursor
}

// formatStreamingWithMarkdown returns streaming text with markdown rendering and cursor
func formatStreamingWithMarkdown(text string, width int) string {
	prefix := aiPrefixStyle.Render("AI:")
	cursor := streamingCursorStyle.Render("▋")
	if text == "" {
		return prefix + " " + cursor
	}

	// Render markdown if content contains markdown
	rendered := renderIfMarkdown(text, width-6)

	return prefix + " " + rendered + cursor
}

// formatToolStatus formats a tool call status line
func formatToolStatus(toolName string, args string) string {
	if args != "" {
		return toolStatusStyle.Render(fmt.Sprintf("🔧 Using tool: %s(%s)", toolName, args))
	}
	return toolStatusStyle.Render(fmt.Sprintf("🔧 Using tool: %s", toolName))
}

// formatTimestamp formats a relative timestamp
func formatTimestamp(t time.Time) string {
	return timestampStyle.Render(relativeTime(t))
}

// relativeTime returns a human-readable relative time string
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// formatProgressBar creates a token usage progress bar
func formatProgressBar(used, total int, width int) string {
	if total <= 0 {
		total = 80000 // Default to 80k context
	}

	percent := float64(used) / float64(total)
	if percent > 1.0 {
		percent = 1.0
	}

	barWidth := width - 30 // Leave room for label and stats
	if barWidth < 10 {
		barWidth = 10
	}

	filled := int(float64(barWidth) * percent)
	empty := barWidth - filled

	bar := progressFilledStyle.Render(strings.Repeat("█", filled)) +
		progressEmptyStyle.Render(strings.Repeat("░", empty))

	stats := fmt.Sprintf("%d%% (%dk/%dk)", int(percent*100), used/1000, total/1000)

	statsStyled := lipgloss.NewStyle().Foreground(dimColor).Render(stats)
	return fmt.Sprintf("Context: %s %s", bar, statsStyled)
}

// renderHeader creates the rich header box
func renderHeader(model string, responseTime time.Duration, width int) string {
	// Format response time
	timeStr := ""
	if responseTime > 0 {
		if responseTime < time.Second {
			timeStr = fmt.Sprintf("%dms", responseTime.Milliseconds())
		} else {
			timeStr = fmt.Sprintf("%.1fs", responseTime.Seconds())
		}
	}

	// Build header content
	parts := []string{"🫀 FeelPulse"}
	if model != "" {
		// Shorten model name for display
		shortModel := model
		if len(model) > 20 {
			shortModel = model[:17] + "..."
		}
		parts = append(parts, shortModel)
	}
	if timeStr != "" {
		parts = append(parts, timeStr)
	}

	content := strings.Join(parts, "  │  ")

	return headerBoxStyle.Width(width - 4).Align(lipgloss.Center).Render(content)
}

// formatKeyboardShortcuts returns the keyboard shortcuts help line
func formatKeyboardShortcuts() string {
	shortcuts := []struct {
		key  string
		desc string
	}{
		{"Enter", "send"},
		{"Ctrl+L", "clear"},
		{"Ctrl+R", "retry"},
		{"Ctrl+C", "quit"},
		{"/", "commands"},
	}

	var parts []string
	for _, s := range shortcuts {
		parts = append(parts, keyStyle.Render(s.key)+" "+keyDescStyle.Render(s.desc))
	}

	return strings.Join(parts, "  ")
}
