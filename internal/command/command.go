package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/memory"
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
	ResetAllSessions() error
}

// SubAgentInfo holds info about a sub-agent
type SubAgentInfo struct {
	ID     string
	Label  string
	Task   string
	Status string
	Result string
	Error  string
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
	sessions      *session.Store
	scheduler     *scheduler.Scheduler
	usage         *usage.Tracker
	skills        *skills.Manager
	memory        *memory.Manager
	cfg           *config.Config
	browser       BrowserNavigator
	compactor     ContextCompactor
	admin         AdminProvider
	subagents     SubAgentProvider
	pins          PinProvider
	activeSession map[string]string // userKey -> active session key
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

// SetMemoryManager sets the memory manager for workspace access
func (h *Handler) SetMemoryManager(m *memory.Manager) {
	h.memory = m
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
	case "skill":
		response = h.handleSkill(args)
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
