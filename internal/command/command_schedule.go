package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/scheduler"
)

// handleRemind creates a reminder
func (h *Handler) handleRemind(ch, userID, args string) string {
	if h.scheduler == nil {
		return "❌ Reminders are not enabled."
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return "⏰ *Usage:* `/remind in <duration> <message>`\n\n*Examples:*\n  `/remind in 10m check email`\n  `/remind in 1h call mom`\n  `/remind in 2d submit report`\n\n*Durations:* `m`=minutes, `h`=hours, `d`=days"
	}

	durationStr, message, err := scheduler.ParseRemindCommand(args)
	if err != nil {
		return fmt.Sprintf("❌ %v\n\n*Usage:* `/remind in <duration> <message>`\n\n*Examples:*\n  `/remind in 10m check email`\n  `/remind in 1h call mom`", err)
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

// handleCancel cancels a reminder by ID
func (h *Handler) handleCancel(ch, userID, args string) string {
	if h.scheduler == nil {
		return "❌ Reminders are not enabled."
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return "❌ Usage: `/cancel <id>`\n\nUse `/reminders` to see reminder IDs."
	}

	// Try to find and cancel the reminder
	reminders := h.scheduler.List(ch, userID)
	for _, r := range reminders {
		// Match by full ID or prefix
		if r.ID == args || strings.HasPrefix(r.ID, args) {
			if h.scheduler.Cancel(r.ID) {
				shortID := r.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				return fmt.Sprintf("✅ Cancelled reminder: [%s] %s", shortID, r.Message)
			}
		}
	}

	return fmt.Sprintf("❌ Reminder not found: `%s`\n\nUse `/reminders` to see active reminders.", args)
}
