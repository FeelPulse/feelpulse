package command

import (
	"context"
	"fmt"
	"strings"
)

// handleAdmin handles admin commands
func (h *Handler) handleAdmin(ch, userID, username, args string) string {
	if h.admin == nil {
		return "❌ Admin commands are not available."
	}

	// Check if user is admin
	adminUsername := h.admin.GetAdminUsername()
	if adminUsername != "" && username != adminUsername {
		return "❌ Access denied. Admin only."
	}

	// Parse subcommand
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	subcmd := strings.ToLower(parts[0])

	switch subcmd {
	case "stats":
		return h.handleAdminStats()
	case "sessions":
		return h.handleAdminSessions()
	case "reload":
		return h.handleAdminReload()
	case "":
		return h.handleAdminHelp()
	default:
		return fmt.Sprintf("❓ Unknown admin command: %s\n\n%s", subcmd, h.handleAdminHelp())
	}
}

// handleAdminStats returns system statistics
func (h *Handler) handleAdminStats() string {
	stats := h.admin.GetSystemStats()

	var sb strings.Builder
	sb.WriteString("📊 *System Statistics*\n\n")
	sb.WriteString(fmt.Sprintf("⏱ Uptime: %v\n", stats["uptime"]))
	sb.WriteString(fmt.Sprintf("🔄 Goroutines: %v\n", stats["goroutines"]))
	sb.WriteString(fmt.Sprintf("💾 Memory: %v MB (alloc) / %v MB (sys)\n",
		stats["memory_alloc_mb"], stats["memory_sys_mb"]))
	sb.WriteString(fmt.Sprintf("📂 Sessions: %v\n", stats["sessions"]))
	sb.WriteString(fmt.Sprintf("🔧 GC cycles: %v\n", stats["gc_cycles"]))

	return sb.String()
}

// handleAdminSessions returns all active sessions
func (h *Handler) handleAdminSessions() string {
	sessions := h.admin.GetAllSessions()

	if len(sessions) == 0 {
		return "📭 No active sessions."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📂 *All Sessions* (%d)\n\n", len(sessions)))

	for i, sess := range sessions {
		if i >= 20 {
			sb.WriteString(fmt.Sprintf("\n... and %d more", len(sessions)-20))
			break
		}

		timeAgo := formatTimeAgo(sess.UpdatedAt)
		msgCount := sess.Len()
		sb.WriteString(fmt.Sprintf("• `%s` — %d msgs, updated %s\n",
			sess.Key, msgCount, timeAgo))
	}

	return sb.String()
}

// handleAdminReload reloads config and workspace files
func (h *Handler) handleAdminReload() string {
	if err := h.admin.ReloadConfig(context.Background()); err != nil {
		return fmt.Sprintf("❌ Reload failed: %v", err)
	}
	return "✅ Configuration and workspace files reloaded."
}

// handleAdminHelp shows admin commands
func (h *Handler) handleAdminHelp() string {
	return `🔐 *Admin Commands*

  /admin stats — System statistics
  /admin sessions — All active sessions  
  /admin reload — Reload config + workspace`
}
