package command

import (
	"fmt"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// handleBrowse navigates to a URL, takes a screenshot, and returns the content
// Returns text and metadata with screenshot_path for Telegram to send as photo
func (h *Handler) handleBrowse(ch, userID, args string) *types.Message {
	args = strings.TrimSpace(args)
	if args == "" {
		return &types.Message{
			Text:    "❌ *Usage:* `/browse <url>`\n\nExample:\n  `/browse https://example.com`\n\nThis fetches the page content and takes a screenshot.",
			Channel: ch,
			IsBot:   true,
		}
	}

	if h.browser == nil {
		return &types.Message{
			Text:    "❌ Browser tools are not enabled.\n\nEnable browser in config:\n```yaml\nbrowser:\n  enabled: true\n```",
			Channel: ch,
			IsBot:   true,
		}
	}

	// Add https:// if no scheme provided
	url := args
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// Navigate and get content
	result, err := h.browser.Navigate(map[string]interface{}{"url": url})
	if err != nil {
		return &types.Message{
			Text:    fmt.Sprintf("❌ Failed to browse: %v", err),
			Channel: ch,
			IsBot:   true,
		}
	}

	// Truncate long results
	const maxLen = 3000
	if len(result) > maxLen {
		result = result[:maxLen] + "\n\n... (truncated)"
	}

	// Take screenshot
	screenshotPath, screenshotErr := h.browser.Screenshot(map[string]interface{}{"url": url})

	response := &types.Message{
		Text:     fmt.Sprintf("🌐 *Page content:*\n\n%s", result),
		Channel:  ch,
		IsBot:    true,
		Metadata: make(map[string]any),
	}

	// Add screenshot path to metadata if successful
	if screenshotErr == nil && screenshotPath != "" {
		response.Metadata["screenshot_path"] = screenshotPath
		response.Metadata["screenshot_caption"] = fmt.Sprintf("📸 Screenshot of %s", url)
	}

	return response
}

// handleCompact manually triggers context compaction
func (h *Handler) handleCompact(ch, userID string) string {
	if h.compactor == nil {
		return "❌ Context compaction is not enabled."
	}

	sess, exists := h.sessions.Get(ch, userID)
	if !exists || sess.Len() == 0 {
		return "📭 No conversation to compact."
	}

	messages := sess.GetAllMessages()
	originalCount := len(messages)
	originalTokens := session.EstimateHistoryTokens(messages)

	compacted, err := h.compactor.ForceCompact(messages)
	if err != nil {
		return fmt.Sprintf("❌ Compaction failed: %v", err)
	}

	if len(compacted) >= originalCount {
		return "ℹ️ Nothing to compact — conversation is already compact."
	}

	// Update session with compacted history
	sess.ReplaceHistory(compacted)

	newTokens := session.EstimateHistoryTokens(compacted)
	return fmt.Sprintf("📦 *Compacted conversation*\n\n"+
		"Messages: %d → %d\n"+
		"Est. tokens: ~%dk → ~%dk\n\n"+
		"Older messages have been summarized.",
		originalCount, len(compacted),
		originalTokens/1000, newTokens/1000)
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

// handleTTS toggles text-to-speech for the session
func (h *Handler) handleTTS(ch, userID, args string) string {
	sess := h.sessions.GetOrCreate(ch, userID)
	args = strings.ToLower(strings.TrimSpace(args))

	switch args {
	case "on", "enable", "true", "1":
		sess.SetTTS(true)
		return "🔊 *TTS Enabled*\n\nBot responses will now be spoken aloud."
	case "off", "disable", "false", "0":
		sess.SetTTS(false)
		return "🔇 *TTS Disabled*\n\nBot responses will be text-only."
	case "":
		// Show current status
		tts := sess.GetTTS()
		if tts == nil {
			return "🔊 *TTS Status:* using global config\n\nUse `/tts on` or `/tts off` to toggle."
		}
		if *tts {
			return "🔊 *TTS Status:* enabled\n\nUse `/tts off` to disable."
		}
		return "🔇 *TTS Status:* disabled\n\nUse `/tts on` to enable."
	default:
		return "❌ Invalid option. Use `/tts on` or `/tts off`."
	}
}
