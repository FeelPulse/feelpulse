package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Handler processes slash commands
type Handler struct {
	sessions *session.Store
	cfg      *config.Config
}

// NewHandler creates a new command handler
func NewHandler(sessions *session.Store, cfg *config.Config) *Handler {
	return &Handler{
		sessions: sessions,
		cfg:      cfg,
	}
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

// handleHelp shows available commands
func (h *Handler) handleHelp() string {
	return `🫀 *FeelPulse Commands*

/new — Start a new conversation (clear history)
/history [n] — Show last n messages (default: 10)
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
