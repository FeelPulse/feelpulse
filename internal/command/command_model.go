package command

import (
	"fmt"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/session"
)

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
		// Show available models inline
		models := session.SupportedModels()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("❌ Unknown model: `%s`\n\n*Available models:*\n", model))
		for _, m := range models {
			sb.WriteString(fmt.Sprintf("  • %s\n", channel.FormatModelName(m)))
		}
		sb.WriteString("\nUse `/model <name>` to switch.")
		return sb.String(), nil
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

// handleProfile manages personality profiles
func (h *Handler) handleProfile(ch, userID, args string) string {
	sess := h.sessions.GetOrCreate(ch, userID)
	args = strings.TrimSpace(args)

	// Parse subcommand
	parts := strings.SplitN(args, " ", 2)
	subcmd := strings.ToLower(parts[0])

	switch subcmd {
	case "list":
		profiles := h.cfg.Workspace.Profiles
		if len(profiles) == 0 {
			return "📭 No profiles configured.\n\nAdd profiles to your config:\n```yaml\nworkspace:\n  profiles:\n    friendly: /path/to/friendly-soul.md\n    formal: /path/to/formal-soul.md\n```"
		}

		var sb strings.Builder
		sb.WriteString("🎭 *Available Profiles*\n\n")
		current := sess.GetProfile()
		for name := range profiles {
			marker := ""
			if name == current {
				marker = " ✓"
			}
			sb.WriteString(fmt.Sprintf("• `%s`%s\n", name, marker))
		}
		sb.WriteString("\nUse `/profile use <name>` to switch.")
		return sb.String()

	case "use", "set", "switch":
		if len(parts) < 2 {
			// Show available profiles in usage hint
			profiles := h.cfg.Workspace.Profiles
			if len(profiles) == 0 {
				return "❌ Usage: `/profile use <name>`\n\nNo profiles configured. Add profiles to your config.yaml."
			}
			var sb strings.Builder
			sb.WriteString("❌ Usage: `/profile use <name>`\n\n*Available profiles:*\n")
			for name := range profiles {
				sb.WriteString(fmt.Sprintf("  • `%s`\n", name))
			}
			return sb.String()
		}
		name := strings.TrimSpace(parts[1])

		// Check if profile exists
		profiles := h.cfg.Workspace.Profiles
		if _, ok := profiles[name]; !ok {
			return fmt.Sprintf("❌ Unknown profile: `%s`\n\nUse `/profile list` to see available profiles.", name)
		}

		sess.SetProfile(name)
		return fmt.Sprintf("✅ Switched to profile: *%s*", name)

	case "reset", "clear", "default":
		sess.SetProfile("")
		return "✅ Reset to default profile."

	case "":
		// Show current profile
		current := sess.GetProfile()
		if current == "" {
			return "🎭 *Current Profile:* default\n\nUse `/profile list` to see available profiles."
		}
		return fmt.Sprintf("🎭 *Current Profile:* %s\n\nUse `/profile list` to see all profiles.", current)

	default:
		return "❓ Unknown subcommand. Use:\n• `/profile list` — show available profiles\n• `/profile use <name>` — switch to a profile\n• `/profile reset` — reset to default"
	}
}
