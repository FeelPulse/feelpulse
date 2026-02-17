package command

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

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

// handleFork creates a fork of the current conversation
func (h *Handler) handleFork(ch, userID, args string) string {
	args = strings.TrimSpace(args)

	// Generate fork name if not provided
	forkName := args
	if forkName == "" {
		b := make([]byte, 4)
		rand.Read(b)
		forkName = hex.EncodeToString(b)
	}

	// Validate fork name
	if strings.ContainsAny(forkName, ":/ ") {
		return "❌ Invalid fork name. Use alphanumeric characters only."
	}

	// Create fork
	_, err := h.sessions.Fork(ch, userID, forkName)
	if err != nil {
		return fmt.Sprintf("❌ Failed to fork: %v", err)
	}

	// Track active session
	userKey := session.SessionKey(ch, userID)
	h.activeSession[userKey] = userKey + ":" + forkName

	return fmt.Sprintf("🔀 *Conversation forked!*\n\nNew branch: `%s`\n\nYou're now on the forked conversation.\nUse `/switch main` to go back to the original.", forkName)
}

// handleSessions lists all sessions for the user
func (h *Handler) handleSessions(ch, userID string) string {
	entries := h.sessions.ListUserSessions(ch, userID)

	if len(entries) == 0 {
		return "📭 No sessions found."
	}

	// Determine active session
	userKey := session.SessionKey(ch, userID)
	activeKey := h.activeSession[userKey]
	if activeKey == "" {
		activeKey = userKey // Default is main
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📂 *Your Sessions* (%d)\n\n", len(entries)))

	for _, entry := range entries {
		active := ""
		if entry.SessionID == activeKey {
			active = " ✓"
		}

		timeAgo := formatTimeAgo(entry.UpdatedAt)
		sb.WriteString(fmt.Sprintf("• `%s`%s — updated %s\n", entry.Name, active, timeAgo))
	}

	sb.WriteString("\n_Use `/switch <name>` to switch sessions._")
	return sb.String()
}

// handleSwitch switches to a different session
func (h *Handler) handleSwitch(ch, userID, args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return "❌ Usage: `/switch <session-name>`\n\nUse `/sessions` to list available sessions."
	}

	_, err := h.sessions.SwitchSession(ch, userID, args)
	if err != nil {
		return fmt.Sprintf("❌ %v\n\nUse `/sessions` to see available sessions.", err)
	}

	// Update active session tracker
	userKey := session.SessionKey(ch, userID)
	if args == "main" {
		h.activeSession[userKey] = userKey
	} else {
		h.activeSession[userKey] = userKey + ":" + args
	}

	return fmt.Sprintf("✅ Switched to session: *%s*", args)
}
