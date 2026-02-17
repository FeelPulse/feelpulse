package command

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/session"
)

// handleUsage shows token usage statistics
func (h *Handler) handleUsage(ch, userID string) string {
	if h.usage == nil {
		return "❌ Usage tracking is not enabled."
	}

	stats := h.usage.Get(ch, userID)
	return stats.String()
}

// handleHelp shows available commands
func (h *Handler) handleHelp() string {
	return `🫀 *FeelPulse — AI Chat Assistant*

📝 *Conversation*
  /new — Start a new conversation
  /clear — Alias for /new
  /history [N] — Show recent messages (default 10)
  /export — Export conversation as .txt file
  /compact — Manually compress conversation history

🔀 *Session Branching*
  /fork [name] — Create a conversation fork
  /sessions — List all your sessions
  /switch <name> — Switch to a different session

📌 *Pins*
  /pin <text> — Pin info to your session
  /pins — List pinned items
  /unpin <id> — Remove a pin

🤖 *AI Model*
  /model — Show or switch AI model
  /models — List available models

🎭 *Personality*
  /profile — Show current profile
  /profile list — List available profiles
  /profile use <name> — Switch profile
  /profile reset — Reset to default

🌐 *Browser*
  /browse <url> — Fetch page content

🔊 *Voice*
  /tts — Show TTS status
  /tts on — Enable text-to-speech
  /tts off — Disable text-to-speech

🛠️ *Skills*
  /skills — List loaded AI tools

🤖 *Sub-agents*
  /agents — List spawned sub-agents
  /agent <id> — Show sub-agent details

⏰ *Reminders*
  /remind in <time> <msg> — Set reminder
  /reminders — List active reminders
  /cancel <id> — Cancel a reminder

📊 *Stats*
  /usage — Show token usage & context

🔐 *Admin*
  /admin — Admin commands (restricted)

❓ *Help*
  /help — Show this message

_Just send any message to chat with the AI!_`
}

// handleAgents lists all sub-agents
func (h *Handler) handleAgents() string {
	if h.subagents == nil {
		return "❌ Sub-agents are not available."
	}

	agents := h.subagents.ListSubAgents()
	if len(agents) == 0 {
		return "📭 No sub-agents have been spawned.\n\nSub-agents are background AI workers. They can be spawned via tool calls."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 *Sub-agents* (%d)\n\n", len(agents)))

	for _, agent := range agents {
		status := formatAgentStatus(agent.Status)
		task := agent.Task
		if len(task) > 50 {
			task = task[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("• `%s` (%s) — %s\n  Task: %s\n\n", agent.ID, agent.Label, status, task))
	}

	sb.WriteString("_Use `/agent <id>` for details._")
	return sb.String()
}

// handleAgent shows details for a specific sub-agent
func (h *Handler) handleAgent(args string) string {
	if h.subagents == nil {
		return "❌ Sub-agents are not available."
	}

	agentID := strings.TrimSpace(args)
	if agentID == "" {
		return "❌ Usage: `/agent <id>`\n\nUse `/agents` to list all sub-agents."
	}

	agent, exists := h.subagents.GetSubAgent(agentID)
	if !exists {
		return fmt.Sprintf("❌ Sub-agent not found: `%s`", agentID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 *Sub-agent: %s* (`%s`)\n\n", agent.Label, agent.ID))
	sb.WriteString(fmt.Sprintf("📋 *Task:* %s\n", agent.Task))
	sb.WriteString(fmt.Sprintf("📊 *Status:* %s\n", formatAgentStatus(agent.Status)))

	if agent.Status == "done" && agent.Result != "" {
		result := agent.Result
		if len(result) > 2000 {
			result = result[:1997] + "..."
		}
		sb.WriteString(fmt.Sprintf("\n📝 *Result:*\n%s", result))
	}

	if agent.Error != "" {
		sb.WriteString(fmt.Sprintf("\n❌ *Error:* %s", agent.Error))
	}

	return sb.String()
}

// handlePin pins text to the current session
func (h *Handler) handlePin(ch, userID, args string) string {
	if h.pins == nil {
		return "❌ Pins are not available."
	}

	text := strings.TrimSpace(args)
	if text == "" {
		return "📌 *Usage:* `/pin <text>`\n\nPins text to your session. Pinned text is always included in the AI's context.\n\n*Examples:*\n  `/pin My name is Alice`\n  `/pin I prefer concise answers`\n  `/pin Always format code in markdown`"
	}

	sessionKey := session.SessionKey(ch, userID)
	pinID, err := h.pins.AddPin(sessionKey, text)
	if err != nil {
		return fmt.Sprintf("❌ Failed to create pin: %v", err)
	}

	shortID := pinID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	return fmt.Sprintf("📌 *Pinned!* (ID: `%s`)\n\n\"%s\"\n\n_This info will be included in every AI response._", shortID, text)
}

// handlePins lists all pinned items for the session
func (h *Handler) handlePins(ch, userID string) string {
	if h.pins == nil {
		return "❌ Pins are not available."
	}

	sessionKey := session.SessionKey(ch, userID)
	pins := h.pins.ListPins(sessionKey)

	if len(pins) == 0 {
		return "📌 No pinned items.\n\nUse `/pin <text>` to pin information to your session."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📌 *Pinned Items* (%d)\n\n", len(pins)))

	for i, pin := range pins {
		shortID := pin.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		text := pin.Text
		if len(text) > 50 {
			text = text[:47] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. `%s` — %s\n", i+1, shortID, text))
	}

	sb.WriteString("\n_Use `/unpin <id>` to remove a pin._")
	return sb.String()
}

// handleUnpin removes a pinned item
func (h *Handler) handleUnpin(ch, userID, args string) string {
	if h.pins == nil {
		return "❌ Pins are not available."
	}

	pinID := strings.TrimSpace(args)
	if pinID == "" {
		return "❌ Usage: `/unpin <id>`\n\nUse `/pins` to see pin IDs."
	}

	sessionKey := session.SessionKey(ch, userID)
	pins := h.pins.ListPins(sessionKey)

	// Find matching pin by ID or prefix
	var foundPin *PinInfo
	for _, pin := range pins {
		if pin.ID == pinID || strings.HasPrefix(pin.ID, pinID) {
			p := pin // capture loop variable
			foundPin = &p
			break
		}
	}

	if foundPin == nil {
		return fmt.Sprintf("❌ Pin not found: `%s`\n\nUse `/pins` to see your pinned items.", pinID)
	}

	if err := h.pins.RemovePin(foundPin.ID); err != nil {
		return fmt.Sprintf("❌ Failed to remove pin: %v", err)
	}

	shortID := foundPin.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	text := foundPin.Text
	if len(text) > 30 {
		text = text[:27] + "..."
	}

	return fmt.Sprintf("🗑️ Pin removed: `%s` — \"%s\"", shortID, text)
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

// formatAgentStatus returns emoji-formatted status for sub-agents
func formatAgentStatus(status string) string {
	switch status {
	case "pending":
		return "⏳ Pending"
	case "running":
		return "🔄 Running"
	case "done":
		return "✅ Done"
	case "failed":
		return "❌ Failed"
	case "canceled":
		return "🚫 Canceled"
	default:
		return status
	}
}

// formatTimeAgo returns a human-readable time ago string
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
