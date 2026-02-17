package tui

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	primaryColor   = lipgloss.Color("#FF6B9D") // Pink/magenta for FeelPulse branding
	secondaryColor = lipgloss.Color("#7C3AED") // Purple accent
	userColor      = lipgloss.Color("#3B82F6") // Blue for user messages
	aiColor        = lipgloss.Color("#10B981") // Green for AI messages
	dimColor       = lipgloss.Color("#6B7280") // Gray for help text
	errorColor     = lipgloss.Color("#EF4444") // Red for errors
)

// Styles
var (
	// Header bar style
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
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
