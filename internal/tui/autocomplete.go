package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// CommandSuggestion represents a slash command with its description
type CommandSuggestion struct {
	Command     string
	Description string
}

// Available slash commands
var availableCommands = []CommandSuggestion{
	{Command: "/new", Description: "Start fresh conversation"},
	{Command: "/model", Description: "Switch AI model"},
	{Command: "/usage", Description: "Show token stats"},
	{Command: "/help", Description: "Show help"},
	{Command: "/quit", Description: "Exit FeelPulse"},
}

// filterCommands filters commands based on prefix
func filterCommands(prefix string) []CommandSuggestion {
	if prefix == "" || prefix == "/" {
		return availableCommands
	}

	prefix = strings.ToLower(prefix)
	var matches []CommandSuggestion
	for _, cmd := range availableCommands {
		if strings.HasPrefix(strings.ToLower(cmd.Command), prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// Autocomplete manages command autocomplete state
type Autocomplete struct {
	active      bool
	suggestions []CommandSuggestion
	selected    int
	prefix      string
}

// NewAutocomplete creates a new autocomplete instance
func NewAutocomplete() *Autocomplete {
	return &Autocomplete{}
}

// Update updates autocomplete state based on input
func (a *Autocomplete) Update(input string) {
	if strings.HasPrefix(input, "/") && !strings.Contains(input, " ") {
		a.active = true
		a.prefix = input
		a.suggestions = filterCommands(input)
		if a.selected >= len(a.suggestions) {
			a.selected = 0
		}
	} else {
		a.active = false
		a.suggestions = nil
		a.selected = 0
	}
}

// IsActive returns true if autocomplete popup should be shown
func (a *Autocomplete) IsActive() bool {
	return a.active && len(a.suggestions) > 0
}

// Next selects the next suggestion
func (a *Autocomplete) Next() {
	if len(a.suggestions) == 0 {
		return
	}
	a.selected = (a.selected + 1) % len(a.suggestions)
}

// Prev selects the previous suggestion
func (a *Autocomplete) Prev() {
	if len(a.suggestions) == 0 {
		return
	}
	a.selected--
	if a.selected < 0 {
		a.selected = len(a.suggestions) - 1
	}
}

// Selected returns the currently selected command
func (a *Autocomplete) Selected() string {
	if len(a.suggestions) == 0 {
		return ""
	}
	return a.suggestions[a.selected].Command
}

// Reset clears autocomplete state
func (a *Autocomplete) Reset() {
	a.active = false
	a.suggestions = nil
	a.selected = 0
	a.prefix = ""
}

// View renders the autocomplete popup
func (a *Autocomplete) View(width int) string {
	if !a.IsActive() {
		return ""
	}

	// Styles for the popup
	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(secondaryColor).
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Background(secondaryColor).
		Foreground(lipgloss.Color("#FFFFFF"))

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	descStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	var lines []string
	for i, cmd := range a.suggestions {
		line := cmd.Command
		// Pad command to align descriptions
		for len(line) < 10 {
			line += " "
		}
		line += " " + cmd.Description

		if i == a.selected {
			lines = append(lines, selectedStyle.Render(line))
		} else {
			// Style command and description separately
			cmdPart := normalStyle.Render(cmd.Command)
			descPart := descStyle.Render("  " + cmd.Description)
			lines = append(lines, cmdPart+descPart)
		}
	}

	content := strings.Join(lines, "\n")
	return popupStyle.Render(content)
}
