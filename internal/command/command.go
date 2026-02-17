package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/scheduler"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/skills"
	"github.com/FeelPulse/feelpulse/internal/usage"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Handler processes slash commands
type Handler struct {
	sessions  *session.Store
	scheduler *scheduler.Scheduler
	usage     *usage.Tracker
	skills    *skills.Manager
	cfg       *config.Config
}

// NewHandler creates a new command handler
func NewHandler(sessions *session.Store, cfg *config.Config) *Handler {
	return &Handler{
		sessions: sessions,
		cfg:      cfg,
	}
}

// SetScheduler sets the scheduler for reminder commands
func (h *Handler) SetScheduler(s *scheduler.Scheduler) {
	h.scheduler = s
}

// SetUsageTracker sets the usage tracker
func (h *Handler) SetUsageTracker(t *usage.Tracker) {
	h.usage = t
}

// SetSkillsManager sets the skills manager
func (h *Handler) SetSkillsManager(m *skills.Manager) {
	h.skills = m
}

// IsCommand checks if a message is a slash command
func IsCommand(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) < 2 {
		return false
	}
	if text[0] != '/' {
		return false
	}
	// Must have a letter after the slash
	if text[1] < 'a' || text[1] > 'z' {
		if text[1] < 'A' || text[1] > 'Z' {
			return false
		}
	}
	return true
}

// ParseCommand extracts the command name and arguments
func ParseCommand(text string) (cmd string, args string) {
	text = strings.TrimSpace(text)
	if !IsCommand(text) {
		return "", ""
	}

	// Remove leading slash
	text = text[1:]

	// Split on first space
	parts := strings.SplitN(text, " ", 2)
	cmd = strings.ToLower(parts[0])

	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return cmd, args
}

// Handle processes a command message and returns a response
func (h *Handler) Handle(msg *types.Message) (*types.Message, error) {
	cmd, args := ParseCommand(msg.Text)
	userID := getUserID(msg)

	var response string
	var keyboard any

	switch cmd {
	case "new", "reset", "clear":
		response, keyboard = h.handleNew(msg.Channel, userID)
	case "history":
		response = h.handleHistory(msg.Channel, userID, args)
	case "remind":
		response = h.handleRemind(msg.Channel, userID, args)
	case "reminders":
		response = h.handleReminders(msg.Channel, userID)
	case "usage", "stats":
		response = h.handleUsage(msg.Channel, userID)
	case "model":
		response, keyboard = h.handleModel(msg.Channel, userID, args)
	case "models":
		response, keyboard = h.handleModels()
	case "skills":
		response = h.handleSkills()
	case "export":
		return h.handleExport(msg.Channel, userID, msg)
	case "help", "start":
		response = h.handleHelp()
	default:
		response = fmt.Sprintf("❓ Unknown command: /%s\n\nType /help for available commands.", cmd)
	}

	return &types.Message{
		Text:     response,
		Channel:  msg.Channel,
		IsBot:    true,
		Keyboard: keyboard,
	}, nil
}

// handleNew clears the session
func (h *Handler) handleNew(ch, userID string) (string, any) {
	h.sessions.Delete(ch, userID)
	keyboard := channel.NewChatKeyboard()
	return "🔄 *Conversation cleared!* Starting fresh.\n\nSend a message to begin your new conversation.", keyboard
}

// handleHistory shows recent messages
func (h *Handler) handleHistory(ch, userID, args string) string {
	sess, exists := h.sessions.Get(ch, userID)
	if !exists || sess.Len() == 0 {
		return "📭 No conversation history yet."
	}

	// Parse limit from args
	limit := 10
	if args != "" {
		if n, err := strconv.Atoi(args); err == nil && n > 0 {
			limit = n
		}
	}

	messages := sess.GetHistory(limit)
	if len(messages) == 0 {
		return "📭 No conversation history yet."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📜 *Conversation History* (%d messages)\n\n", len(messages)))

	for _, m := range messages {
		role := "👤"
		if m.IsBot {
			role = "🤖"
		}

		timeStr := ""
		if !m.Timestamp.IsZero() {
			timeStr = m.Timestamp.Format(time.Kitchen)
		}

		// Truncate long messages
		text := m.Text
		if len(text) > 100 {
			text = text[:97] + "..."
		}

		if timeStr != "" {
			sb.WriteString(fmt.Sprintf("%s [%s] %s\n", role, timeStr, text))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s\n", role, text))
		}
	}

	return sb.String()
}

// handleRemind creates a reminder
func (h *Handler) handleRemind(ch, userID, args string) string {
	if h.scheduler == nil {
		return "❌ Reminders are not enabled."
	}

	durationStr, message, err := scheduler.ParseRemindCommand(args)
	if err != nil {
		return fmt.Sprintf("❌ %v\n\nUsage: /remind in <duration> <message>\nExamples:\n  /remind in 10m check email\n  /remind in 1h call mom\n  /remind in 2d submit report", err)
	}

	duration, err := scheduler.ParseDuration(durationStr)
	if err != nil {
		return fmt.Sprintf("❌ Invalid duration: %s", durationStr)
	}

	id, err := h.scheduler.AddReminder(ch, userID, duration, message)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create reminder: %v", err)
	}

	fireAt := time.Now().Add(duration)
	return fmt.Sprintf("⏰ Reminder set!\nID: %s\nFires at: %s\nMessage: %s", 
		id[:8], fireAt.Format(time.RFC822), message)
}

// handleReminders lists active reminders
func (h *Handler) handleReminders(ch, userID string) string {
	if h.scheduler == nil {
		return "❌ Reminders are not enabled."
	}

	reminders := h.scheduler.List(ch, userID)
	if len(reminders) == 0 {
		return "📭 No active reminders."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⏰ *Active Reminders* (%d)\n\n", len(reminders)))
	for _, r := range reminders {
		sb.WriteString(r.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

// handleUsage shows token usage statistics
func (h *Handler) handleUsage(ch, userID string) string {
	if h.usage == nil {
		return "❌ Usage tracking is not enabled."
	}

	stats := h.usage.Get(ch, userID)
	return stats.String()
}

// handleModel switches the model for the current session
func (h *Handler) handleModel(ch, userID, args string) (string, any) {
	sess := h.sessions.GetOrCreate(ch, userID)
	keyboard := channel.ModelKeyboard()
	
	// If no argument, show current model with keyboard
	if args == "" {
		current := sess.GetModel()
		if current == "" {
			return "🤖 *Current Model:* default\n\nSelect a model below or type `/model <name>`:", keyboard
		}
		return fmt.Sprintf("🤖 *Current Model:* %s\n\nSelect a model below or type `/model <name>`:", channel.FormatModelName(current)), keyboard
	}

	// Validate and set new model
	model := strings.TrimSpace(args)
	if !session.ValidateModel(model) {
		return fmt.Sprintf("❌ Unknown model: %s\n\nUse /models to see available options.", model), nil
	}

	sess.SetModel(model)
	return fmt.Sprintf("✅ Model switched to: *%s*", channel.FormatModelName(model)), nil
}

// handleModels lists available models
func (h *Handler) handleModels() (string, any) {
	models := session.SupportedModels()
	keyboard := channel.ModelKeyboard()
	
	var sb strings.Builder
	sb.WriteString("🤖 *Available Models*\n\n")
	for _, m := range models {
		sb.WriteString(fmt.Sprintf("  • %s\n", channel.FormatModelName(m)))
	}
	sb.WriteString("\nTap a button below or use `/model <name>`:")
	return sb.String(), keyboard
}

// handleSkills lists loaded skills
func (h *Handler) handleSkills() string {
	if h.skills == nil {
		return "❌ Skills system not enabled."
	}

	loaded := h.skills.ListSkills()
	if len(loaded) == 0 {
		return "📭 No skills loaded.\n\nSkills can be added to the workspace/skills directory."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🛠️ *Loaded Skills* (%d)\n\n", len(loaded)))

	for _, skill := range loaded {
		hasExec := ""
		if skill.Executable != "" {
			hasExec = " ⚡"
		}
		sb.WriteString(fmt.Sprintf("• *%s*%s\n", skill.Name, hasExec))
		if skill.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", skill.Description))
		}
	}

	return sb.String()
}

// handleExport exports conversation history as a text file
func (h *Handler) handleExport(ch, userID string, msg *types.Message) (*types.Message, error) {
	sess, exists := h.sessions.Get(ch, userID)
	if !exists || sess.Len() == 0 {
		return &types.Message{
			Text:    "📭 No conversation to export.",
			Channel: ch,
			IsBot:   true,
		}, nil
	}

	messages := sess.GetAllMessages()

	// Build export content
	var sb strings.Builder
	sb.WriteString("# FeelPulse Conversation Export\n")
	sb.WriteString(fmt.Sprintf("# Exported: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("# Messages: %d\n\n", len(messages)))

	for _, m := range messages {
		role := "User"
		if m.IsBot {
			role = "AI"
		}

		timestamp := m.Timestamp.Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n\n", timestamp, role, m.Text))
	}

	content := sb.String()

	// Return special message with export data
	return &types.Message{
		Text:    content,
		Channel: ch,
		IsBot:   true,
		Metadata: map[string]any{
			"export":   true,
			"filename": fmt.Sprintf("feelpulse-export-%s.txt", time.Now().Format("2006-01-02")),
			"chat_id":  msg.Metadata["chat_id"],
		},
	}, nil
}

// FormatExport formats messages for export (public helper for testing)
func FormatExport(messages []types.Message) string {
	var sb strings.Builder
	sb.WriteString("# FeelPulse Conversation Export\n")
	sb.WriteString(fmt.Sprintf("# Exported: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("# Messages: %d\n\n", len(messages)))

	for _, m := range messages {
		role := "User"
		if m.IsBot {
			role = "AI"
		}

		timestamp := m.Timestamp.Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n\n", timestamp, role, m.Text))
	}

	return sb.String()
}

// handleHelp shows available commands
func (h *Handler) handleHelp() string {
	return `🫀 *FeelPulse — AI Chat Assistant*

📝 *Conversation*
  /new — Start a new conversation
  /history — Show recent messages
  /export — Export conversation as .txt file

🤖 *AI Model*
  /model — Show or switch AI model
  /models — List available models

🛠️ *Skills*
  /skills — List loaded AI tools

⏰ *Reminders*
  /remind in <time> <msg> — Set reminder
  /reminders — List active reminders

📊 *Stats*
  /usage — Show token usage

❓ *Help*
  /help — Show this message

_Just send any message to chat with the AI!_`
}

// HandleCallback processes inline keyboard button presses
func (h *Handler) HandleCallback(ch string, userID int64, action, value string) (string, *channel.InlineKeyboard, error) {
	uid := strconv.FormatInt(userID, 10)
	
	switch action {
	case "model":
		// User selected a model from the keyboard
		if !session.ValidateModel(value) {
			return fmt.Sprintf("❌ Unknown model: %s", value), nil, nil
		}
		sess := h.sessions.GetOrCreate(ch, uid)
		sess.SetModel(value)
		return fmt.Sprintf("✅ Model switched to: *%s*", channel.FormatModelName(value)), nil, nil
		
	case "new":
		// Confirmation tap on new chat button - just acknowledge
		return "🔄 Chat cleared! Send a message to continue.", nil, nil
		
	default:
		return "", nil, nil
	}
}

// getUserID extracts user ID from message metadata
func getUserID(msg *types.Message) string {
	if msg.Metadata != nil {
		if userID, ok := msg.Metadata["user_id"]; ok {
			switch v := userID.(type) {
			case string:
				return v
			case int64:
				return strconv.FormatInt(v, 10)
			case int:
				return strconv.Itoa(v)
			case float64:
				return strconv.FormatInt(int64(v), 10)
			}
		}
	}
	if msg.From != "" {
		return msg.From
	}
	return "unknown"
}
