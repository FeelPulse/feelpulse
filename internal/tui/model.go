package tui

import (
	"fmt"
	"strings"
	"time"

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

// Default context window size (Claude 3.5 Sonnet)
const defaultContextSize = 200000

// streamMsg is sent when streaming text arrives
type streamMsg struct {
	delta string
	done  bool
	err   error
}

// toolCallMsg is sent when a tool is being called
type toolCallMsg struct {
	name   string
	args   string
	active bool
}

// TimestampedMessage wraps a message with timestamp
type TimestampedMessage struct {
	types.Message
	Timestamp time.Time
}

// Model is the bubbletea model for the TUI
type Model struct {
	viewport     viewport.Model
	textarea     textarea.Model
	messages     []TimestampedMessage
	config       *config.Config
	agent        *agent.Router
	session      *session.Session
	store        *session.Store
	usage        *usage.Tracker
	thinking     bool
	streaming    bool
	streamText   string
	err          error
	width        int
	height       int
	ready        bool
	quitting     bool
	currentModel string

	// Enhanced features
	autocomplete     *Autocomplete
	responseTime     time.Duration
	responseStart    time.Time
	currentTool      string
	currentToolArgs  string
	inputTokens      int
	outputTokens     int
	totalTokens      int
	contextSize      int
	lastUserMessage  string
	tickCount        int
}

// responseMsg is sent when AI responds (streaming complete)
type responseMsg struct {
	text  string
	err   error
	usage types.Usage
	model string
}

// tickMsg for blinking cursor animation
type tickMsg time.Time

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
	ta.Placeholder = "Type your message... (Enter to send)"
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
		messages:     []TimestampedMessage{},
		currentModel: cfg.Agent.Model,
		autocomplete: NewAutocomplete(),
		contextSize:  defaultContextSize,
	}, nil
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.tick())
}

// tick returns a command that sends tick messages for cursor animation
func (m Model) tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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

		case tea.KeyCtrlL:
			// Clear conversation
			m.session.Clear()
			m.messages = []TimestampedMessage{}
			m.inputTokens = 0
			m.outputTokens = 0
			m.totalTokens = 0
			m.addSystemMessage("Started new conversation")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, nil

		case tea.KeyCtrlR:
			// Retry last message
			if !m.thinking && !m.streaming && m.lastUserMessage != "" {
				return m.handleInput(m.lastUserMessage)
			}
			return m, nil

		case tea.KeyTab:
			// Autocomplete selection
			if m.autocomplete.IsActive() {
				selected := m.autocomplete.Selected()
				m.textarea.SetValue(selected)
				m.textarea.CursorEnd()
				m.autocomplete.Reset()
				return m, nil
			}
			// Tab cycles through suggestions
			if m.autocomplete.IsActive() {
				m.autocomplete.Next()
				return m, nil
			}

		case tea.KeyUp:
			// Navigate autocomplete or scroll
			if m.autocomplete.IsActive() {
				m.autocomplete.Prev()
				return m, nil
			}

		case tea.KeyDown:
			// Navigate autocomplete
			if m.autocomplete.IsActive() {
				m.autocomplete.Next()
				return m, nil
			}

		case tea.KeyEsc:
			// Close autocomplete
			if m.autocomplete.IsActive() {
				m.autocomplete.Reset()
				return m, nil
			}

		case tea.KeyEnter:
			// If autocomplete active, select it
			if m.autocomplete.IsActive() {
				selected := m.autocomplete.Selected()
				m.textarea.SetValue(selected + " ")
				m.textarea.CursorEnd()
				m.autocomplete.Reset()
				return m, nil
			}

			// If shift is held, let textarea handle it (newline)
			if msg.Alt {
				break
			}
			// Submit message
			if !m.thinking && !m.streaming {
				text := strings.TrimSpace(m.textarea.Value())
				if text != "" {
					m.textarea.Reset()
					m.autocomplete.Reset()
					return m.handleInput(text)
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3 // Rich header box
		inputHeight := 5  // textarea + help line
		statusHeight := 2 // tool status + context bar
		chatHeight := m.height - headerHeight - inputHeight - statusHeight - 2

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

	case streamMsg:
		if msg.err != nil {
			m.streaming = false
			m.thinking = false
			m.err = msg.err
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}

		if msg.done {
			// Streaming complete
			m.streaming = false
			m.responseTime = time.Since(m.responseStart)
			return m, nil
		}

		// Accumulate streaming text
		m.streamText += msg.delta
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case toolCallMsg:
		if msg.active {
			m.currentTool = msg.name
			m.currentToolArgs = msg.args
		} else {
			m.currentTool = ""
			m.currentToolArgs = ""
		}
		return m, nil

	case responseMsg:
		m.thinking = false
		m.streaming = false
		m.responseTime = time.Since(m.responseStart)

		if msg.err != nil {
			m.err = msg.err
		} else {
			// Add AI response to messages
			aiMsg := TimestampedMessage{
				Message: types.Message{
					Text:  msg.text,
					IsBot: true,
				},
				Timestamp: time.Now(),
			}
			m.messages = append(m.messages, aiMsg)
			m.session.AddMessage(aiMsg.Message)

			// Record usage
			m.usage.Record(tuiChannel, tuiUserID, msg.usage.InputTokens, msg.usage.OutputTokens, msg.model)
			m.inputTokens += msg.usage.InputTokens
			m.outputTokens += msg.usage.OutputTokens
			m.totalTokens = m.inputTokens + m.outputTokens
		}

		m.streamText = ""
		m.currentTool = ""
		m.currentToolArgs = ""
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case tickMsg:
		m.tickCount++
		// Only re-render if streaming (for cursor blink)
		if m.streaming {
			m.viewport.SetContent(m.renderMessages())
		}
		return m, m.tick()
	}

	// Handle textarea updates
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	// Update autocomplete based on input
	m.autocomplete.Update(m.textarea.Value())

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
	m.lastUserMessage = text
	userMsg := TimestampedMessage{
		Message: types.Message{
			Text:    text,
			IsBot:   false,
			Channel: tuiChannel,
		},
		Timestamp: time.Now(),
	}
	m.messages = append(m.messages, userMsg)
	m.session.AddMessage(userMsg.Message)
	m.thinking = true
	m.streaming = true
	m.streamText = ""
	m.responseStart = time.Now()
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	// Send to AI asynchronously with streaming
	return m, m.sendToAIStream()
}

// handleCommand processes slash commands
func (m Model) handleCommand(cmd Command, arg string) (tea.Model, tea.Cmd) {
	switch cmd {
	case CmdNew:
		m.session.Clear()
		m.messages = []TimestampedMessage{}
		m.inputTokens = 0
		m.outputTokens = 0
		m.totalTokens = 0
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
	m.messages = append(m.messages, TimestampedMessage{
		Message: types.Message{
			Text:  text,
			IsBot: true,
			Metadata: map[string]any{
				"system": true,
			},
		},
		Timestamp: time.Now(),
	})
}

// sendToAIStream sends the conversation to the AI with streaming
func (m Model) sendToAIStream() tea.Cmd {
	return func() tea.Msg {
		// Get all messages from session
		messages := m.session.GetAllMessages()

		// Create streaming callback that sends deltas to TUI
		// Note: In real implementation, we'd use a channel to send
		// deltas back to the bubbletea program. For now, we'll
		// collect and send as a single response.

		// Call the agent with streaming
		resp, err := m.agent.ProcessWithHistoryStream(messages, func(delta string) {
			// In a full implementation, we'd send this via a channel
			// to the bubbletea program. For now, this accumulates in the agent.
		})

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
			if mdl, ok := resp.Metadata["model"].(string); ok {
				model = mdl
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

	// Rich header
	header := renderHeader(m.currentModel, m.responseTime, m.width)
	b.WriteString(header)
	b.WriteString("\n")

	// Chat viewport
	chatContent := chatBorderStyle.Width(m.width - 2).Render(m.viewport.View())
	b.WriteString(chatContent)
	b.WriteString("\n")

	// Tool status line (if active)
	if m.currentTool != "" {
		toolStatus := formatToolStatus(m.currentTool, m.currentToolArgs)
		b.WriteString(toolStatus)
		b.WriteString("\n")
	}

	// Context progress bar
	contextBar := formatProgressBar(m.totalTokens, m.contextSize, m.width-4)
	b.WriteString(contextBar)
	b.WriteString("\n")

	// Autocomplete popup (if active)
	if m.autocomplete.IsActive() {
		popup := m.autocomplete.View(m.width - 4)
		b.WriteString(popup)
		b.WriteString("\n")
	}

	// Input area
	inputBox := inputBorderStyle.Width(m.width - 2).Render(m.textarea.View())
	b.WriteString(inputBox)
	b.WriteString("\n")

	// Keyboard shortcuts help bar
	help := formatKeyboardShortcuts()
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
	if len(m.messages) == 0 && !m.streaming {
		return systemStyle.Render("Welcome to FeelPulse TUI! Type a message to start chatting.\n\nCommands: /new, /model, /usage, /help, /quit")
	}

	var lines []string
	for _, msg := range m.messages {
		formatted := m.formatTimestampedMessage(msg)
		wrapped := wrapText(formatted, m.viewport.Width-4)
		lines = append(lines, wrapped)
		lines = append(lines, "") // Add blank line between messages
	}

	// Add streaming text if active
	if m.streaming {
		// Show blinking cursor based on tick count
		showCursor := m.tickCount%2 == 0
		if showCursor {
			lines = append(lines, formatStreamingWithMarkdown(m.streamText, m.viewport.Width-4))
		} else {
			if m.streamText == "" {
				lines = append(lines, aiPrefixStyle.Render("AI:")+" ")
			} else {
				prefix := aiPrefixStyle.Render("AI:")
				rendered := renderIfMarkdown(m.streamText, m.viewport.Width-10)
				lines = append(lines, prefix+" "+rendered)
			}
		}
	} else if m.thinking {
		lines = append(lines, formatThinking())
	}

	return strings.Join(lines, "\n")
}

// formatTimestampedMessage formats a single message with timestamp
func (m Model) formatTimestampedMessage(msg TimestampedMessage) string {
	// Check if it's a system message
	if msg.Metadata != nil {
		if sys, ok := msg.Metadata["system"].(bool); ok && sys {
			return formatSystemMessage(msg.Text)
		}
	}

	// Format message content
	var content string
	if msg.IsBot {
		// AI messages get markdown rendering
		content = formatAIMessageWithMarkdown(msg.Text, m.viewport.Width-4)
	} else {
		content = formatUserMessage(msg.Text)
	}

	// Add timestamp
	if !msg.Timestamp.IsZero() {
		timestamp := formatTimestamp(msg.Timestamp)
		content = content + "  " + timestamp
	}

	return content
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
  /quit        Exit the TUI

Keyboard shortcuts:
  Enter        Send message
  Shift+Enter  New line
  Ctrl+L       Clear conversation
  Ctrl+R       Retry last message
  Ctrl+C       Quit
  Tab          Select autocomplete
  ↑/↓          Navigate autocomplete`
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
