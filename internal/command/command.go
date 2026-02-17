package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/scheduler"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/usage"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Handler processes slash commands
type Handler struct {
	sessions  *session.Store
	scheduler *scheduler.Scheduler
	usage     *usage.Tracker
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

	switch cmd {
	case "new", "reset", "clear":
		response = h.handleNew(msg.Channel, userID)
	case "history":
		response = h.handleHistory(msg.Channel, userID, args)
	case "remind":
		response = h.handleRemind(msg.Channel, userID, args)
	case "reminders":
		response = h.handleReminders(msg.Channel, userID)
	case "usage", "stats":
		response = h.handleUsage(msg.Channel, userID)
	case "model":
		response = h.handleModel(msg.Channel, userID, args)
	case "models":
		response = h.handleModels()
	case "help", "start":
		response = h.handleHelp()
	default:
		response = fmt.Sprintf("❓ Unknown command: /%s\n\nType /help for available commands.", cmd)
	}

	return &types.Message{
		Text:    response,
		Channel: msg.Channel,
		IsBot:   true,
	}, nil
}

// handleNew clears the session
func (h *Handler) handleNew(channel, userID string) string {
	h.sessions.Delete(channel, userID)
	return "🔄 Conversation cleared. Starting fresh!"
}

// handleHistory shows recent messages
func (h *Handler) handleHistory(channel, userID, args string) string {
	sess, exists := h.sessions.Get(channel, userID)
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
func (h *Handler) handleRemind(channel, userID, args string) string {
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

	id, err := h.scheduler.AddReminder(channel, userID, duration, message)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create reminder: %v", err)
	}

	fireAt := time.Now().Add(duration)
	return fmt.Sprintf("⏰ Reminder set!\nID: %s\nFires at: %s\nMessage: %s", 
		id[:8], fireAt.Format(time.RFC822), message)
}

// handleReminders lists active reminders
func (h *Handler) handleReminders(channel, userID string) string {
	if h.scheduler == nil {
		return "❌ Reminders are not enabled."
	}

	reminders := h.scheduler.List(channel, userID)
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
func (h *Handler) handleUsage(channel, userID string) string {
	if h.usage == nil {
		return "❌ Usage tracking is not enabled."
	}

	stats := h.usage.Get(channel, userID)
	return stats.String()
}

// handleModel switches the model for the current session
func (h *Handler) handleModel(channel, userID, args string) string {
	sess := h.sessions.GetOrCreate(channel, userID)
	
	// If no argument, show current model
	if args == "" {
		current := sess.GetModel()
		if current == "" {
			return "🤖 Using default model from config.\n\nTo switch: /model <name>\nList models: /models"
		}
		return fmt.Sprintf("🤖 Current model: *%s*\n\nTo switch: /model <name>\nList models: /models", current)
	}

	// Validate and set new model
	model := strings.TrimSpace(args)
	if !session.ValidateModel(model) {
		return fmt.Sprintf("❌ Unknown model: %s\n\nUse /models to see available options.", model)
	}

	sess.SetModel(model)
	return fmt.Sprintf("✅ Model switched to: *%s*", model)
}

// handleModels lists available models
func (h *Handler) handleModels() string {
	models := session.SupportedModels()
	
	var sb strings.Builder
	sb.WriteString("🤖 *Available Models*\n\n")
	sb.WriteString("*Anthropic Claude:*\n")
	for _, m := range models {
		if strings.HasPrefix(m, "claude") {
			sb.WriteString(fmt.Sprintf("  • %s\n", m))
		}
	}
	sb.WriteString("\n*OpenAI GPT:*\n")
	for _, m := range models {
		if strings.HasPrefix(m, "gpt") || strings.HasPrefix(m, "o1") {
			sb.WriteString(fmt.Sprintf("  • %s\n", m))
		}
	}
	sb.WriteString("\nSwitch: /model <name>")
	return sb.String()
}

// handleHelp shows available commands
func (h *Handler) handleHelp() string {
	return `🫀 *FeelPulse Commands*

/new — Start a new conversation (clear history)
/history [n] — Show last n messages (default: 10)
/model [name] — Show or switch AI model
/models — List available models
/remind in <time> <message> — Set a reminder
/reminders — List active reminders
/usage — Show token usage statistics
/help — Show this help message

Just send a message to chat with the AI!`
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
