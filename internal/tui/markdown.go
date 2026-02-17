package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

// markdownIndicators are patterns that suggest content contains markdown
var markdownIndicators = regexp.MustCompile("(?m)(^```|^#{1,6}\\s|^[*-]\\s|\\*\\*|__|`[^`]+`|^>\\s|^\\d+\\.\\s)")

// containsMarkdown checks if text likely contains markdown formatting
func containsMarkdown(text string) bool {
	return markdownIndicators.MatchString(text)
}

// renderMarkdown renders markdown content with glamour
func renderMarkdown(content string, width int) string {
	if width < 40 {
		width = 80
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return content
	}

	out, err := r.Render(content)
	if err != nil {
		return content
	}

	// Trim excessive trailing newlines from glamour output
	return strings.TrimRight(out, "\n")
}

// renderIfMarkdown renders content through glamour only if it contains markdown
func renderIfMarkdown(content string, width int) string {
	if containsMarkdown(content) {
		return renderMarkdown(content, width)
	}
	return content
}
