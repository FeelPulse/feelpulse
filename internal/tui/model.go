package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/usage"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Command represents a slash command
type Command int

const (
	CmdNone Command = iota
	CmdNew
	CmdModel
	CmdUsage
	CmdHelp
	CmdQuit
	CmdUnknown
)

// TUI session identifiers
const (
	tuiChannel = "tui"
	tuiUserID  = "local"
)

// Model is the bubbletea model for the TUI
type Model struct {
	viewport     viewport.Model
	textarea     textarea.Model
	messages     []types.Message
	config       *config.Config
	agent        *agent.Router
	session      *session.Session
	store        *session.Store
	usage        *usage.Tracker
	thinking     bool
	err          error
	width        int
	height       int
	ready        bool
	quitting     bool
	currentModel string
}

// responseMsg is sent when AI responds
type responseMsg struct {
	text  string
	err   error
	usage types.Usage
	model string
}

// New creates a new TUI model
func New(cfg *config.Config) (*Model, error) {
	// Initialize agent router
	router, err := agent.NewRouter(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize agent: %w", err)
	}

	// Initialize session store and get/create session
	store := session.NewStore()
	sess := store.GetOrCreate(tuiChannel, tuiUserID)

	// Initialize usage tracker
	usageTracker := usage.NewTracker()

	// Create textarea for input
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send, Shift+Enter for newline)"
	ta.Focus()
	ta.CharLimit = 4096
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(true) // Shift+Enter for newline

	return &Model{
		textarea:     ta,
		config:       cfg,
		agent:        router,
		session:      sess,
		store:        store,
		usage:        usageTracker,
		messages:     []types.Message{},
		currentModel: cfg.Agent.Model,
	}, nil
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			// If shift is held, let textarea handle it (newline)
			if msg.Alt {
				break
			}
			// Submit message
			if !m.thinking {
				text := strings.TrimSpace(m.textarea.Value())
				if text != "" {
					m.textarea.Reset()
					return m.handleInput(text)
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 1
		inputHeight := 5 // textarea + help line
		chatHeight := m.height - headerHeight - inputHeight - 4

		if !m.ready {
			m.viewport = viewport.New(m.width-2, chatHeight)
			m.viewport.SetContent(m.renderMessages())
			m.ready = true
		} else {
			m.viewport.Width = m.width - 2
			m.viewport.Height = chatHeight
		}

		m.textarea.SetWidth(m.width - 4)
		return m, nil

	case responseMsg:
		m.thinking = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Add AI response to messages
			aiMsg := types.Message{
				Text:  msg.text,
				IsBot: true,
			}
			m.messages = append(m.messages, aiMsg)
			m.session.AddMessage(aiMsg)

			// Record usage
			m.usage.Record(tuiChannel, tuiUserID, msg.usage.InputTokens, msg.usage.OutputTokens, msg.model)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Handle textarea updates
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	// Handle viewport updates
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleInput processes user input (message or command)
func (m Model) handleInput(text string) (tea.Model, tea.Cmd) {
	// Check for commands
	if isCommand(text) {
		cmd, arg := parseCommand(text)
		return m.handleCommand(cmd, arg)
	}

	// Regular message - add to history and send to AI
	userMsg := types.Message{
		Text:    text,
		IsBot:   false,
		Channel: tuiChannel,
	}
	m.messages = append(m.messages, userMsg)
	m.session.AddMessage(userMsg)
	m.thinking = true
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	// Send to AI asynchronously
	return m, m.sendToAI()
}

// handleCommand processes slash commands
func (m Model) handleCommand(cmd Command, arg string) (tea.Model, tea.Cmd) {
	switch cmd {
	case CmdNew:
		m.session.Clear()
		m.messages = []types.Message{}
		m.addSystemMessage("Started new conversation")

	case CmdModel:
		if arg == "" {
			// Show current model and available models
			models := session.SupportedModels()
			msg := fmt.Sprintf("Current model: %s\nAvailable: %s", m.currentModel, strings.Join(models, ", "))
			m.addSystemMessage(msg)
		} else {
			if session.ValidateModel(arg) {
				m.currentModel = arg
				m.session.SetModel(arg)
				m.addSystemMessage(fmt.Sprintf("Model set to: %s", arg))
			} else {
				m.addSystemMessage(fmt.Sprintf("Unknown model: %s. Use /model to see available models.", arg))
			}
		}

	case CmdUsage:
		stats := m.usage.Get(tuiChannel, tuiUserID)
		m.addSystemMessage(stats.String())

	case CmdHelp:
		m.addSystemMessage(helpText())

	case CmdQuit:
		m.quitting = true
		return m, tea.Quit

	case CmdUnknown:
		m.addSystemMessage("Unknown command. Type /help for available commands.")
	}

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// addSystemMessage adds a system message to the chat
func (m *Model) addSystemMessage(text string) {
	m.messages = append(m.messages, types.Message{
		Text:  text,
		IsBot: true,
		Metadata: map[string]any{
			"system": true,
		},
	})
}

// sendToAI sends the conversation to the AI and returns a response
func (m Model) sendToAI() tea.Cmd {
	return func() tea.Msg {
		// Get all messages from session
		messages := m.session.GetAllMessages()

		// Call the agent
		resp, err := m.agent.ProcessWithHistory(messages)
		if err != nil {
			return responseMsg{err: err}
		}

		// Extract usage from metadata
		var u types.Usage
		if resp.Metadata != nil {
			if in, ok := resp.Metadata["input_tokens"].(int); ok {
				u.InputTokens = in
			}
			if out, ok := resp.Metadata["output_tokens"].(int); ok {
				u.OutputTokens = out
			}
		}

		model := m.currentModel
		if resp.Metadata != nil {
			if m, ok := resp.Metadata["model"].(string); ok {
				model = m
			}
		}

		return responseMsg{
			text:  resp.Text,
			usage: u,
			model: model,
		}
	}
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting {
		return "Goodbye! 👋\n"
	}

	if !m.ready {
		return "Initializing..."
	}

	// Build the UI
	var b strings.Builder

	// Header
	header := headerStyle.Width(m.width).Render(fmt.Sprintf("  🫀 FeelPulse  [model: %s]", m.currentModel))
	b.WriteString(header)
	b.WriteString("\n")

	// Chat viewport
	chatContent := chatBorderStyle.Width(m.width - 2).Render(m.viewport.View())
	b.WriteString(chatContent)
	b.WriteString("\n")

	// Input area
	inputBox := inputBorderStyle.Width(m.width - 2).Render(m.textarea.View())
	b.WriteString(inputBox)
	b.WriteString("\n")

	// Help bar
	help := helpStyle.Render("[Enter] send  [Shift+Enter] newline  [Ctrl+C] quit  [/help] commands")
	b.WriteString(help)

	// Error display
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(formatError(m.err.Error()))
	}

	return b.String()
}

// renderMessages renders all messages for the viewport
func (m Model) renderMessages() string {
	if len(m.messages) == 0 {
		return systemStyle.Render("Welcome to FeelPulse TUI! Type a message to start chatting.\n\nCommands: /new, /model, /usage, /help, /quit")
	}

	var lines []string
	for _, msg := range m.messages {
		formatted := formatMessage(msg)
		wrapped := wrapText(formatted, m.viewport.Width-4)
		lines = append(lines, wrapped)
		lines = append(lines, "") // Add blank line between messages
	}

	// Add thinking indicator if waiting
	if m.thinking {
		lines = append(lines, formatThinking())
	}

	return strings.Join(lines, "\n")
}

// formatMessage formats a single message
func formatMessage(msg types.Message) string {
	// Check if it's a system message
	if msg.Metadata != nil {
		if sys, ok := msg.Metadata["system"].(bool); ok && sys {
			return formatSystemMessage(msg.Text)
		}
	}

	if msg.IsBot {
		return formatAIMessage(msg.Text)
	}
	return formatUserMessage(msg.Text)
}

// parseCommand parses a slash command and its argument
func parseCommand(input string) (Command, string) {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "/") {
		return CmdNone, ""
	}

	parts := strings.SplitN(input, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/new":
		return CmdNew, arg
	case "/model":
		return CmdModel, arg
	case "/usage":
		return CmdUsage, arg
	case "/help":
		return CmdHelp, arg
	case "/quit", "/exit", "/q":
		return CmdQuit, arg
	default:
		return CmdUnknown, arg
	}
}

// isCommand checks if input is a slash command
func isCommand(input string) bool {
	return strings.HasPrefix(input, "/")
}

// helpText returns the help message
func helpText() string {
	return `Available commands:
  /new         Start a new conversation (clears history)
  /model       Show current model and available models
  /model NAME  Switch to a different model
  /usage       Show token usage statistics
  /help        Show this help message
  /quit        Exit the TUI`
}

// wrapText wraps text to fit within the given width
func wrapText(text string, width int) string {
	if width <= 0 {
		width = 80
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Simple word wrap
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if lipgloss.Width(currentLine+" "+word) <= width {
				currentLine += " " + word
			} else {
				result.WriteString(currentLine)
				result.WriteString("\n")
				currentLine = word
			}
		}
		result.WriteString(currentLine)
	}

	return result.String()
}

// Run starts the TUI
func Run(cfg *config.Config) error {
	model, err := New(cfg)
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
