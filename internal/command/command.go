package command

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// BrowserNavigator interface for /browse command
type BrowserNavigator interface {
	Navigate(params map[string]interface{}) (string, error)
	Screenshot(params map[string]interface{}) (string, error)
}

// ContextCompactor interface for /compact command
type ContextCompactor interface {
	CompactIfNeeded(messages []types.Message) ([]types.Message, error)
	ForceCompact(messages []types.Message) ([]types.Message, error)
}

// AdminProvider interface for admin commands
type AdminProvider interface {
	GetAdminUsername() string
	GetSystemStats() map[string]any
	GetAllSessions() []*session.Session
	ReloadConfig(ctx context.Context) error
}

// SubAgentInfo holds info about a sub-agent
type SubAgentInfo struct {
	ID        string
	Label     string
	Task      string
	Status    string
	Result    string
	Error     string
}

// SubAgentProvider interface for /agents command
type SubAgentProvider interface {
	ListSubAgents() []SubAgentInfo
	GetSubAgent(id string) (*SubAgentInfo, bool)
	CancelSubAgent(id string) error
}

// PinInfo holds info about a pinned item
type PinInfo struct {
	ID        string
	Text      string
	CreatedAt time.Time
}

// PinProvider interface for /pin commands
type PinProvider interface {
	AddPin(sessionKey, text string) (string, error)
	ListPins(sessionKey string) []PinInfo
	RemovePin(id string) error
	GetPins(sessionKey string) string // Returns combined pins text for system prompt
}

// Handler processes slash commands
type Handler struct {
	sessions       *session.Store
	scheduler      *scheduler.Scheduler
	usage          *usage.Tracker
	skills         *skills.Manager
	cfg            *config.Config
	browser        BrowserNavigator
	compactor      ContextCompactor
	admin          AdminProvider
	subagents      SubAgentProvider
	pins           PinProvider
	activeSession  map[string]string // userKey -> active session key
}

// NewHandler creates a new command handler
func NewHandler(sessions *session.Store, cfg *config.Config) *Handler {
	return &Handler{
		sessions:      sessions,
		cfg:           cfg,
		activeSession: make(map[string]string),
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

// SetBrowser sets the browser for /browse command
func (h *Handler) SetBrowser(b BrowserNavigator) {
	h.browser = b
}

// SetCompactor sets the compactor for /compact command
func (h *Handler) SetCompactor(c ContextCompactor) {
	h.compactor = c
}

// SetAdmin sets the admin provider for /admin commands
func (h *Handler) SetAdmin(a AdminProvider) {
	h.admin = a
}

// SetSubAgents sets the sub-agent provider for /agents command
func (h *Handler) SetSubAgents(s SubAgentProvider) {
	h.subagents = s
}

// SetPins sets the pin provider for /pin commands
func (h *Handler) SetPins(p PinProvider) {
	h.pins = p
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
	case "cancel":
		response = h.handleCancel(msg.Channel, userID, args)
	case "usage", "stats":
		response = h.handleUsage(msg.Channel, userID)
	case "model":
		response, keyboard = h.handleModel(msg.Channel, userID, args)
	case "models":
		response, keyboard = h.handleModels()
	case "skills":
		response = h.handleSkills()
	case "tts":
		response = h.handleTTS(msg.Channel, userID, args)
	case "profile":
		response = h.handleProfile(msg.Channel, userID, args)
	case "export":
		return h.handleExport(msg.Channel, userID, msg)
	case "browse":
		return h.handleBrowse(msg.Channel, userID, args), nil
	case "compact":
		response = h.handleCompact(msg.Channel, userID)
	case "fork":
		response = h.handleFork(msg.Channel, userID, args)
	case "sessions":
		response = h.handleSessions(msg.Channel, userID)
	case "switch":
		response = h.handleSwitch(msg.Channel, userID, args)
	case "admin":
		response = h.handleAdmin(msg.Channel, userID, msg.From, args)
	case "agents":
		response = h.handleAgents()
	case "agent":
		response = h.handleAgent(args)
	case "pin":
		response = h.handlePin(msg.Channel, userID, args)
	case "pins":
		response = h.handlePins(msg.Channel, userID)
	case "unpin":
		response = h.handleUnpin(msg.Channel, userID, args)
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
			return "❌ Usage: `/profile use <name>`"
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
