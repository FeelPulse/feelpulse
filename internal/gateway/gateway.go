package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/browser"
	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/command"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/heartbeat"
	"github.com/FeelPulse/feelpulse/internal/logger"
	"github.com/FeelPulse/feelpulse/internal/memory"
	"github.com/FeelPulse/feelpulse/internal/metrics"
	"github.com/FeelPulse/feelpulse/internal/ratelimit"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/store"
	"github.com/FeelPulse/feelpulse/internal/subagent"
	"github.com/FeelPulse/feelpulse/internal/tools"
	"github.com/FeelPulse/feelpulse/internal/usage"
	"github.com/FeelPulse/feelpulse/internal/watcher"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

type Gateway struct {
	cfg            *config.Config
	mux            *http.ServeMux
	server         *http.Server
	telegram       *channel.TelegramBot
	router         *agent.Router
	sessions       *session.Store
	db             *store.SQLiteStore
	commands       *command.Handler
	memory         *memory.Manager
	compactor      *session.Compactor
	limiter        *ratelimit.Limiter
	watcher        *watcher.ConfigWatcher
	heartbeat      *heartbeat.Service
	usage          *usage.Tracker
	browser         *browser.Browser
	toolRegistry    *tools.Registry
	subagentManager *subagent.Manager
	pinManager      *pinManager
	log             *logger.Logger
	metrics        *metrics.Collector
	startTime      time.Time
	lastMessageAt  atomic.Int64 // Unix nanoseconds of last message
	cancelCtx      context.CancelFunc
	activeRequests sync.WaitGroup // tracks in-flight message processing
	shutdownCh     chan struct{}  // signals shutdown in progress
	mu             sync.RWMutex   // protects router, telegram, compactor during hot reload
}

func New(cfg *config.Config) *Gateway {
	// Initialize logger
	log := logger.New(&logger.Config{
		Level:     cfg.Log.Level,
		Component: "gateway",
	})
	logger.SetDefaultLogger(log)

	sessions := session.NewStore()

	// Initialize SQLite session persistence
	dbPath := store.DefaultDBPath()
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		log.Warn("Failed to initialize session database: %v", err)
	} else {
		if err := sessions.SetPersister(sqliteStore); err != nil {
			log.Warn("Failed to load persisted sessions: %v", err)
		}
	}

	// Initialize memory/workspace manager
	workspacePath := cfg.Workspace.Path
	if workspacePath == "" {
		workspacePath = memory.DefaultWorkspacePath()
	}
	memMgr := memory.NewManager(workspacePath)
	if err := memMgr.Load(); err != nil {
		log.Warn("Failed to load workspace files: %v", err)
	} else if memMgr.Soul() != "" || memMgr.User() != "" || memMgr.Memory() != "" {
		log.Info("📂 Workspace loaded from %s", workspacePath)
	}

	// Initialize rate limiter
	limiter := ratelimit.New(cfg.Agent.RateLimit)
	if cfg.Agent.RateLimit > 0 {
		log.Info("⏱️  Rate limiting enabled: %d messages/minute", cfg.Agent.RateLimit)
	}

	// Initialize usage tracker
	usageTracker := usage.NewTracker()

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector()

	// Initialize tool registry with built-in tools (exec with security config)
	toolRegistry := tools.NewRegistry()
	execCfg := &tools.ExecConfig{
		Enabled:         cfg.Tools.Exec.Enabled,
		AllowedCommands: cfg.Tools.Exec.AllowedCommands,
		TimeoutSeconds:  cfg.Tools.Exec.TimeoutSeconds,
	}
	tools.RegisterBuiltinsWithExec(toolRegistry, execCfg)
	if cfg.Tools.Exec.Enabled {
		log.Info("🔧 Exec tool enabled with %d allowed commands", len(cfg.Tools.Exec.AllowedCommands))
	}

	// Register file tools (sandboxed to workspace)
	fileCfg := &tools.FileConfig{
		Enabled:       cfg.Tools.File.Enabled,
		WorkspacePath: workspacePath,
	}
	tools.RegisterFileTools(toolRegistry, fileCfg)
	if cfg.Tools.File.Enabled {
		log.Info("📁 File tools enabled (sandboxed to %s)", workspacePath)
	}

	// Register read_skill tool for on-demand skill loading
	if skillNames := memMgr.ListSkillNames(); len(skillNames) > 0 {
		toolRegistry.Register(&tools.Tool{
			Name:        "read_skill",
			Description: "Read the full documentation for a skill. Available skills: " + strings.Join(skillNames, ", "),
			Parameters: []tools.Parameter{
				{Name: "name", Type: "string", Description: "Skill name to read", Required: true},
			},
			Handler: func(ctx context.Context, params map[string]any) (string, error) {
				name, _ := params["name"].(string)
				if name == "" {
					return "", fmt.Errorf("skill name is required")
				}
				return memMgr.ReadSkill(name)
			},
		})
		log.Info("📚 %d skills available (on-demand): %s", len(skillNames), strings.Join(skillNames, ", "))
	}

	gw := &Gateway{
		cfg:          cfg,
		mux:          http.NewServeMux(),
		sessions:     sessions,
		db:           sqliteStore,
		commands:     command.NewHandler(sessions, cfg),
		memory:       memMgr,
		limiter:      limiter,
		usage:        usageTracker,
		toolRegistry: toolRegistry,
		log:          log,
		metrics:      metricsCollector,
		startTime:    time.Now(),
		shutdownCh:   make(chan struct{}),
	}

	// Initialize sub-agent manager (callback set later when telegram is ready)
	gw.subagentManager = subagent.NewManager(nil)
	log.Info("🤖 Sub-agent manager initialized")

	gw.commands.SetUsageTracker(usageTracker)
	gw.setupRoutes()
	return gw
}

func (gw *Gateway) setupRoutes() {
	gw.mux.HandleFunc("/health", gw.handleHealth)
	gw.mux.HandleFunc("/hooks/", gw.handleHook)
	gw.mux.HandleFunc("/v1/chat/completions", gw.handleOpenAIChatCompletion)
	gw.mux.HandleFunc("/dashboard", gw.handleDashboard)
	gw.mux.HandleFunc("/dashboard/config", gw.handleConfigPage)
	gw.mux.HandleFunc("/api/config", gw.handleConfigSave)

	// Metrics endpoint
	if gw.cfg.Metrics.Enabled {
		metricsPath := gw.cfg.Metrics.Path
		if metricsPath == "" {
			metricsPath = "/metrics"
		}
		gw.mux.HandleFunc(metricsPath, gw.handleMetrics)
	}
}

func (gw *Gateway) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	gw.cancelCtx = cancel

	// Initialize agent and telegram
	gw.initializeAgent(ctx)
	gw.initializeTelegram(ctx)

	// Initialize browser automation
	gw.initializeBrowser()

	// Initialize sub-agent system (after agent and telegram are ready)
	gw.initializeSubAgents()

	// Wire up command handler dependencies
	gw.wireCommandHandler()

	// Initialize heartbeat service
	gw.initializeHeartbeat()

	// Start config hot reload watcher
	gw.startConfigWatcher(ctx)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", gw.cfg.Gateway.Bind, gw.cfg.Gateway.Port)
	gw.server = &http.Server{
		Addr:         addr,
		Handler:      gw.mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		gw.gracefulShutdown(cancel)
	}()

	gw.log.Info("🫀 Gateway listening on %s", addr)
	return gw.server.ListenAndServe()
}

// gracefulShutdown performs orderly shutdown of all components
func (gw *Gateway) gracefulShutdown(cancel context.CancelFunc) {
	fmt.Println("\n👋 Shutting down...")

	// Signal shutdown in progress
	close(gw.shutdownCh)

	// Stop accepting new requests from Telegram
	gw.mu.RLock()
	telegram := gw.telegram
	gw.mu.RUnlock()
	if telegram != nil {
		telegram.Stop()
		gw.log.Info("📱 Telegram bot stopped")
	}

	// Wait for active message processing to complete (with timeout)
	gw.log.Info("⏳ Waiting for active requests to complete...")
	done := make(chan struct{})
	go func() {
		gw.activeRequests.Wait()
		close(done)
	}()

	select {
	case <-done:
		gw.log.Info("✅ All active requests completed")
	case <-time.After(30 * time.Second):
		gw.log.Warn("Timeout waiting for requests, forcing shutdown")
	}

	// Stop background services
	if gw.watcher != nil {
		gw.watcher.Stop()
	}
	if gw.heartbeat != nil {
		gw.heartbeat.Stop()
	}

	// Save all sessions to SQLite
	if gw.db != nil {
		savedCount := gw.saveAllSessions()
		if savedCount > 0 {
			gw.log.Info("💾 Sessions saved: %d", savedCount)
		}
	}

	// Close browser
	if gw.browser != nil {
		gw.browser.Close()
		gw.log.Info("🌐 Browser closed")
	}

	// Close database connection
	if gw.db != nil {
		if err := gw.db.Close(); err != nil {
			gw.log.Warn("Error closing database: %v", err)
		} else {
			gw.log.Info("💾 Database connection closed")
		}
	}

	// Cancel context and stop server
	cancel()
	gw.server.Close()

	gw.log.Info("👋 Shutdown complete")
}

// saveAllSessions saves all active sessions to the database
func (gw *Gateway) saveAllSessions() int {
	if gw.db == nil {
		return 0
	}

	sessions := gw.sessions.GetRecent(1000) // Get all sessions
	count := 0

	for _, sess := range sessions {
		messages := sess.GetAllMessages()
		if len(messages) == 0 {
			continue
		}

		model := sess.GetModel()
		profile := sess.GetProfile()

		if err := gw.db.SaveWithProfile(sess.Key, messages, model, profile); err != nil {
			gw.log.Warn("Failed to save session %s: %v", sess.Key, err)
		} else {
			count++
		}
	}

	return count
}

// initializeAgent sets up the agent router and compactor
func (gw *Gateway) initializeAgent(ctx context.Context) {
	if gw.cfg.Agent.APIKey == "" && gw.cfg.Agent.AuthToken == "" {
		return
	}

	router, err := agent.NewRouter(gw.cfg)
	if err != nil {
		gw.log.Warn("Agent not configured: %v", err)
		return
	}

	// Inject workspace files into system prompt
	router.SetSystemPromptBuilder(gw.memory.BuildSystemPrompt)

	// Wire up tool registry for agentic tool calling
	if gw.toolRegistry != nil {
		router.SetToolRegistry(gw.toolRegistry)
		toolCount := len(gw.toolRegistry.List())
		if toolCount > 0 {
			gw.log.Info("🔧 Tool registry attached with %d tools", toolCount)
		}
	}

	gw.mu.Lock()
	gw.router = router
	gw.mu.Unlock()

	gw.log.Info("🤖 Agent initialized: %s/%s", gw.cfg.Agent.Provider, gw.cfg.Agent.Model)

	// Log system prompt on startup
	systemPrompt := gw.memory.BuildSystemPrompt("")
	gw.log.Info("🧠 System prompt:\n%s", systemPrompt)

	// Initialize compactor with summarizer
	if anthropicClient, ok := router.Agent().(*agent.AnthropicClient); ok {
		summarizer := agent.NewConversationSummarizer(anthropicClient)
		maxTokens := gw.cfg.Agent.MaxContextTokens
		if maxTokens <= 0 {
			maxTokens = session.DefaultMaxContextTokens
		}
		gw.mu.Lock()
		gw.compactor = session.NewCompactor(summarizer, maxTokens, session.DefaultKeepLastN)
		gw.mu.Unlock()
		gw.log.Info("📦 Context compaction enabled (threshold: %dk tokens)", maxTokens/1000)
	}
}

// initializeTelegram sets up the Telegram bot
func (gw *Gateway) initializeTelegram(ctx context.Context) {
	if !gw.cfg.Channels.Telegram.Enabled || gw.cfg.Channels.Telegram.BotToken == "" {
		return
	}

	telegram := channel.NewTelegramBot(gw.cfg.Channels.Telegram.BotToken)
	telegram.SetHandler(gw.handleMessage)
	telegram.SetStreamingHandler(gw.handleMessageStreaming)
	telegram.SetCallbackHandler(gw.handleTelegramCallback)
	telegram.SetAllowedUsers(gw.cfg.Channels.Telegram.AllowedUsers)

	if len(gw.cfg.Channels.Telegram.AllowedUsers) > 0 {
		gw.log.Info("🔒 Telegram allowlist: %v", gw.cfg.Channels.Telegram.AllowedUsers)
	}

	if err := telegram.Start(ctx); err != nil {
		gw.log.Warn("Failed to start telegram: %v", err)
		return
	}

	gw.mu.Lock()
	gw.telegram = telegram
	gw.mu.Unlock()

	gw.log.Info("📱 Telegram streaming enabled for responsive UX")
}

// initializeHeartbeat sets up the heartbeat service
func (gw *Gateway) initializeHeartbeat() {
	if !gw.cfg.Heartbeat.Enabled {
		return
	}

	workspacePath := gw.cfg.Workspace.Path
	if workspacePath == "" {
		workspacePath = memory.DefaultWorkspacePath()
	}

	hbCfg := &heartbeat.Config{
		Enabled:         gw.cfg.Heartbeat.Enabled,
		IntervalMinutes: gw.cfg.Heartbeat.IntervalMinutes,
	}

	gw.heartbeat = heartbeat.New(hbCfg, workspacePath)

	// Set callback to send messages via Telegram
	gw.heartbeat.SetCallback(func(ch string, userID int64, message string) {
		gw.mu.RLock()
		telegram := gw.telegram
		gw.mu.RUnlock()

		if telegram != nil && ch == "telegram" {
			if err := telegram.SendMessage(userID, message, true); err != nil {
				gw.log.Warn("Failed to send heartbeat to %d: %v", userID, err)
			} else {
				gw.log.Debug("💓 Sent heartbeat to user %d", userID)
			}
		}
	})

	gw.heartbeat.Start()
}

// initializeBrowser sets up the browser automation tools
func (gw *Gateway) initializeBrowser() {
	if !gw.cfg.Browser.Enabled {
		return
	}

	cfg := &browser.Config{
		Enabled:        gw.cfg.Browser.Enabled,
		Headless:       gw.cfg.Browser.Headless,
		TimeoutSeconds: gw.cfg.Browser.TimeoutSeconds,
		Stealth:        gw.cfg.Browser.Stealth,
	}

	b, err := browser.New(cfg)
	if err != nil {
		gw.log.Warn("Browser tools disabled: %v", err)
		return
	}

	// Set screenshot callback to send to Telegram
	b.SetScreenshotCallback(func(path string) error {
		// This callback is called when a screenshot is taken
		// We'll handle sending in the message handler based on context
		gw.log.Debug("📸 Screenshot saved: %s", path)
		return nil
	})

	gw.browser = b

	// Register browser tools with the tool registry
	if gw.toolRegistry != nil {
		gw.registerBrowserTools(b)
	}

	gw.log.Info("🌐 Browser automation enabled (headless=%v, stealth=%v)", cfg.Headless, cfg.Stealth)
}
// registerBrowserTools registers all browser tools with the tool registry
func (gw *Gateway) registerBrowserTools(b *browser.Browser) {
	b.RegisterTools(func(name, description string, params []browser.ToolParam, handler func(context.Context, map[string]interface{}) (string, error)) {
		toolParams := make([]tools.Parameter, len(params))
		for i, p := range params {
			toolParams[i] = tools.Parameter{
				Name:        p.Name,
				Type:        p.Type,
				Description: p.Description,
				Required:    p.Required,
			}
		}

		gw.toolRegistry.Register(&tools.Tool{
			Name:        name,
			Description: description,
			Parameters:  toolParams,
			Handler:     handler,
		})
	})

	gw.log.Info("🔧 Registered %d browser tools", 6)
}

// wireCommandHandler wires up optional command handler dependencies
func (gw *Gateway) wireCommandHandler() {
	// Wire up browser for /browse command
	if gw.browser != nil {
		gw.commands.SetBrowser(gw.browser)
	}

	// Wire up compactor for /compact command
	gw.mu.RLock()
	compactor := gw.compactor
	gw.mu.RUnlock()

	if compactor != nil {
		gw.commands.SetCompactor(compactor)
	}

	// Wire up admin provider for /admin commands
	gw.commands.SetAdmin(gw)

	// Wire up sub-agent provider for /agents command
	if gw.subagentManager != nil {
		gw.commands.SetSubAgents(gw)
	}

	// Wire up memory manager for /skill commands
	if gw.memory != nil {
		gw.commands.SetMemoryManager(gw.memory)
		gw.commands.SetSkillReloadCallback(gw.reloadSkills)
	}

	// Wire up pin manager for /pin commands
	if gw.db != nil {
		pm, err := newPinManager(gw.db, gw.log)
		if err != nil {
			gw.log.Warn("Failed to initialize pin manager: %v", err)
		} else {
			gw.pinManager = pm
			gw.commands.SetPins(pm)
			gw.log.Info("📌 Pin manager initialized")
		}
	}
}

// reloadSkills hot-reloads skills after install/update
func (gw *Gateway) reloadSkills() error {
	// Reload workspace files (including skills)
	if err := gw.memory.Load(); err != nil {
		return fmt.Errorf("failed to reload skills: %w", err)
	}

	// Re-register read_skill tool with updated skill list
	if skillNames := gw.memory.ListSkillNames(); len(skillNames) > 0 {
		gw.toolRegistry.Register(&tools.Tool{
			Name:        "read_skill",
			Description: "Read the full documentation for a skill. Available skills: " + strings.Join(skillNames, ", "),
			Parameters: []tools.Parameter{
				{Name: "name", Type: "string", Description: "Skill name to read", Required: true},
			},
			Handler: func(ctx context.Context, params map[string]any) (string, error) {
				name, _ := params["name"].(string)
				if name == "" {
					return "", fmt.Errorf("skill name is required")
				}
				return gw.memory.ReadSkill(name)
			},
		})
		gw.log.Info("🔄 Skills reloaded: %s", strings.Join(skillNames, ", "))
	}

	return nil
}

// GetBrowser returns the browser instance (for tool execution)
func (gw *Gateway) GetBrowser() *browser.Browser {
	return gw.browser
}

// handleTelegramCallback processes inline keyboard button presses
func (gw *Gateway) handleTelegramCallback(chatID int64, userID int64, action, value string) (string, *channel.InlineKeyboard, error) {
	return gw.commands.HandleCallback("telegram", userID, action, value)
}

// startConfigWatcher starts watching the config file for changes
func (gw *Gateway) startConfigWatcher(ctx context.Context) {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".feelpulse", "config.yaml")

	gw.watcher = watcher.NewConfigWatcher(configPath, watcher.DefaultPollInterval)
	gw.watcher.SetCallback(func() {
		gw.handleConfigReload(ctx)
	})
	gw.watcher.Start()
	gw.log.Info("👁️  Watching config for changes: %s", configPath)
}

// handleConfigReload handles config file changes
func (gw *Gateway) handleConfigReload(ctx context.Context) {
	newCfg, err := config.Load()
	if err != nil {
		gw.log.Warn("Failed to reload config: %v", err)
		return
	}

	gw.log.Info("🔄 Config reloaded")

	oldCfg := gw.cfg
	gw.cfg = newCfg

	// Update log level if changed
	if oldCfg.Log.Level != newCfg.Log.Level {
		gw.log.SetLevel(logger.ParseLevel(newCfg.Log.Level))
		gw.log.Info("Log level changed to: %s", newCfg.Log.Level)
	}

	// Reload workspace files
	if err := gw.memory.Load(); err != nil {
		gw.log.Warn("Failed to reload workspace files: %v", err)
	}

	// Check if agent needs reinitialization
	agentChanged := oldCfg.Agent.APIKey != newCfg.Agent.APIKey ||
		oldCfg.Agent.AuthToken != newCfg.Agent.AuthToken ||
		oldCfg.Agent.Model != newCfg.Agent.Model ||
		oldCfg.Agent.Provider != newCfg.Agent.Provider

	if agentChanged {
		gw.log.Info("🔄 Reinitializing agent...")
		gw.initializeAgent(ctx)
	}

	// Check if telegram needs reinitialization
	telegramChanged := oldCfg.Channels.Telegram.BotToken != newCfg.Channels.Telegram.BotToken ||
		oldCfg.Channels.Telegram.Enabled != newCfg.Channels.Telegram.Enabled

	if telegramChanged {
		gw.log.Info("🔄 Reinitializing Telegram...")
		gw.mu.Lock()
		oldTelegram := gw.telegram
		gw.telegram = nil
		gw.mu.Unlock()

		if oldTelegram != nil {
			oldTelegram.Stop()
		}
		gw.initializeTelegram(ctx)
	} else {
		// Update allowlist without full restart
		gw.mu.RLock()
		telegram := gw.telegram
		gw.mu.RUnlock()
		if telegram != nil {
			telegram.SetAllowedUsers(newCfg.Channels.Telegram.AllowedUsers)
			if len(newCfg.Channels.Telegram.AllowedUsers) > 0 {
				gw.log.Info("🔒 Telegram allowlist updated: %v", newCfg.Channels.Telegram.AllowedUsers)
			}
		}
	}

	// Update commands handler with new config
	gw.commands = command.NewHandler(gw.sessions, newCfg)
	gw.wireCommandHandler()

	// Update rate limiter if changed
	if oldCfg.Agent.RateLimit != newCfg.Agent.RateLimit {
		gw.limiter = ratelimit.New(newCfg.Agent.RateLimit)
		if newCfg.Agent.RateLimit > 0 {
			gw.log.Info("⏱️  Rate limit updated: %d messages/minute", newCfg.Agent.RateLimit)
		} else {
			gw.log.Info("⏱️  Rate limiting disabled")
		}
	}
}

// messageProcessingContext holds state for message processing
type messageProcessingContext struct {
	userID    string
	reqLog    *logger.ContextLogger
	router    *agent.Router
	history   []types.Message
}

// prepareMessageProcessing handles common setup for message processing.
// Returns a context for further processing, or an early reply if processing should stop.
func (gw *Gateway) prepareMessageProcessing(msg *types.Message) (*messageProcessingContext, *types.Message) {
	// Track active request for graceful shutdown
	gw.activeRequests.Add(1)

	// Check if shutting down
	select {
	case <-gw.shutdownCh:
		gw.activeRequests.Done()
		return nil, &types.Message{
			Text:    "⏳ Service is shutting down. Please try again in a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}
	default:
	}

	// Update last message timestamp (atomic, no lock needed)
	gw.lastMessageAt.Store(time.Now().UnixNano())

	// Track message in metrics
	gw.metrics.IncrementMessages(msg.Channel)
	gw.metrics.SetActiveSessions(gw.sessions.Count())

	userID := gw.getUserID(msg)
	reqLog := gw.log.WithComponent("message").WithRequestID(userID)

	reqLog.Info("Processing message from %s", msg.From)

	// Register user for heartbeat (if enabled)
	if gw.heartbeat != nil {
		if uid, ok := gw.getUserIDInt64(msg); ok {
			gw.heartbeat.RegisterUser(msg.Channel, uid)
		}
	}

	// Check for slash commands first (exempt from rate limiting)
	if command.IsCommand(msg.Text) {
		gw.activeRequests.Done()
		reply, _ := gw.commands.Handle(msg)
		return nil, reply
	}

	// Check rate limit
	if !gw.limiter.Allow(userID) {
		reqLog.Info("Rate limited")
		gw.activeRequests.Done()
		return nil, &types.Message{
			Text:    "⏱ Rate limit exceeded. Please wait a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}
	}

	// Get router with read lock (safe during hot reload)
	gw.mu.RLock()
	router := gw.router
	compactor := gw.compactor
	gw.mu.RUnlock()

	if router == nil {
		gw.activeRequests.Done()
		return nil, &types.Message{
			Text:    "🔧 AI agent not configured. Set your API key in config.yaml",
			Channel: msg.Channel,
			IsBot:   true,
		}
	}

	// Get session
	sess := gw.sessions.GetOrCreate(msg.Channel, userID)

	// Add incoming message to session history (and persist)
	gw.sessions.AddMessageAndPersist(msg.Channel, userID, *msg)

	// Get conversation history for agent
	history := sess.GetAllMessages()

	// Compact history if needed (summarize old messages)
	maxContextTokens := session.DefaultMaxContextTokens
	if compactor != nil {
		compacted, err := compactor.CompactIfNeeded(history)
		if err != nil {
			reqLog.Warn("Compaction failed: %v (using full history)", err)
		} else if len(compacted) < len(history) {
			reqLog.Info("📦 Compacted %d messages → summary (%d messages kept)", len(history)-len(compacted)+1, len(compacted))
			history = compacted
			// Track compaction
			if gw.usage != nil {
				gw.usage.RecordCompaction(msg.Channel, userID)
			}
		}
	}

	// Track context window usage
	if gw.usage != nil {
		contextTokens := session.EstimateHistoryTokens(history)
		gw.usage.UpdateContextWindow(msg.Channel, userID, contextTokens, maxContextTokens)
	}

	return &messageProcessingContext{
		userID:  userID,
		reqLog:  reqLog,
		router:  router,
		history: history,
	}, nil
}

// finalizeMessageProcessing handles post-processing after agent response
func (gw *Gateway) finalizeMessageProcessing(msg *types.Message, ctx *messageProcessingContext, reply *types.Message) {
	// Track token usage in metrics
	if reply.Metadata != nil {
		if input, ok := reply.Metadata["input_tokens"].(int); ok {
			if output, ok := reply.Metadata["output_tokens"].(int); ok {
				gw.metrics.AddTokens(input, output)
			}
		}
	}

	// Add bot reply to session history (and persist)
	gw.sessions.AddMessageAndPersist(msg.Channel, ctx.userID, *reply)
}

// handleMessageStreaming processes messages with streaming support for Telegram
func (gw *Gateway) handleMessageStreaming(msg *types.Message, onDelta func(delta string)) (reply *types.Message, err error) {
	ctx, earlyReply := gw.prepareMessageProcessing(msg)
	if earlyReply != nil {
		return earlyReply, nil
	}
	defer gw.activeRequests.Done() // single Done, always runs

	// Panic recovery - ensure we don't crash from unexpected panics
	defer func() {
		if r := recover(); r != nil {
			ctx.reqLog.Error("panic in handleMessageStreaming: %v", r)
			reply = &types.Message{
				Text:    "❌ An unexpected error occurred. Please try again.",
				Channel: msg.Channel,
				IsBot:   true,
			}
			err = nil // Return gracefully instead of crashing
		}
	}()

	// Route to agent with streaming callback
	reply, err = ctx.router.ProcessWithHistoryStream(ctx.history, agent.StreamCallback(onDelta))
	if err != nil {
		ctx.reqLog.Error("Agent error: %v", err)
		return &types.Message{
			Text:    "❌ Sorry, I encountered an error processing your message.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	gw.finalizeMessageProcessing(msg, ctx, reply)
	return reply, nil
}

// handleMessage processes incoming messages from channels
func (gw *Gateway) handleMessage(msg *types.Message) (reply *types.Message, err error) {
	ctx, earlyReply := gw.prepareMessageProcessing(msg)
	if earlyReply != nil {
		return earlyReply, nil
	}
	defer gw.activeRequests.Done() // single Done, always runs

	// Panic recovery - ensure we don't crash from unexpected panics
	defer func() {
		if r := recover(); r != nil {
			ctx.reqLog.Error("panic in handleMessage: %v", r)
			reply = &types.Message{
				Text:    "❌ An unexpected error occurred. Please try again.",
				Channel: msg.Channel,
				IsBot:   true,
			}
			err = nil // Return gracefully instead of crashing
		}
	}()

	// Route to agent with full history
	reply, err = ctx.router.ProcessWithHistory(ctx.history)
	if err != nil {
		ctx.reqLog.Error("Agent error: %v", err)
		return &types.Message{
			Text:    "❌ Sorry, I encountered an error processing your message.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	gw.finalizeMessageProcessing(msg, ctx, reply)
	return reply, nil
}

// getUserID extracts the user ID from a message
func (gw *Gateway) getUserID(msg *types.Message) string {
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
	// Fallback to From field
	if msg.From != "" {
		return msg.From
	}
	return "unknown"
}

// getUserIDInt64 extracts the user ID as int64 from a message
func (gw *Gateway) getUserIDInt64(msg *types.Message) (int64, bool) {
	if msg.Metadata != nil {
		if userID, ok := msg.Metadata["user_id"]; ok {
			switch v := userID.(type) {
			case int64:
				return v, true
			case int:
				return int64(v), true
			case float64:
				return int64(v), true
			}
		}
	}
	return 0, false
}

func (gw *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	gw.mu.RLock()
	router := gw.router
	telegram := gw.telegram
	browser := gw.browser
	gw.mu.RUnlock()

	// Read last message time atomically
	lastMsgNano := gw.lastMessageAt.Load()
	var lastMsg time.Time
	if lastMsgNano > 0 {
		lastMsg = time.Unix(0, lastMsgNano)
	}

	// Calculate uptime
	uptime := time.Since(gw.startTime)
	uptimeStr := formatDuration(uptime)

	// Count sessions
	sessionCount := gw.sessions.Count()

	// Count registered tools
	toolCount := 0
	if gw.toolRegistry != nil {
		toolCount = len(gw.toolRegistry.List())
	}

	// Check if agent is configured
	agentConfigured := router != nil

	status := map[string]any{
		"ok":               agentConfigured,
		"version":          "0.1.0",
		"uptime":           uptimeStr,
		"uptime_seconds":   int(uptime.Seconds()),
		"sessions_count":   sessionCount,
		"tools_registered": toolCount,
		"browser_available": browser != nil,
		"channels": map[string]bool{
			"telegram": telegram != nil,
		},
	}

	// Add last message timestamp if we've received any messages
	if !lastMsg.IsZero() {
		status["last_message_at"] = lastMsg.Format(time.RFC3339)
	}

	// Add agent info if configured
	if agentConfigured {
		status["agent"] = router.Agent().Name()
	} else {
		status["error"] = "Agent not configured (missing API key or auth token)"
	}

	w.Header().Set("Content-Type", "application/json")

	// Return 503 if agent is not configured
	if !agentConfigured {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// formatDuration formats a duration as human-readable string
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// checkAuth verifies the Authorization header against the configured hooks token.
// Returns true if authorized (or no auth required). Writes 401 and returns false if unauthorized.
func (gw *Gateway) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if gw.cfg.Hooks.Token == "" {
		return true // no auth configured
	}

	// Try Authorization header first (Bearer token)
	authHeader := r.Header.Get("Authorization")
	if authHeader == "Bearer "+gw.cfg.Hooks.Token {
		return true
	}

	// Try query parameter (e.g., ?token=xxx for browser access)
	queryToken := r.URL.Query().Get("token")
	if queryToken == gw.cfg.Hooks.Token {
		return true
	}

	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return false
}

func (gw *Gateway) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !gw.checkAuth(w, r) {
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	gw.log.Info("📨 Hook received: %s", r.URL.Path)

	// TODO: Route to agent based on hook mappings

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
	})
}

// handleMetrics returns Prometheus-compatible metrics
func (gw *Gateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !gw.checkAuth(w, r) {
		return
	}

	// Update active sessions count before returning metrics
	gw.metrics.SetActiveSessions(gw.sessions.Count())

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	gw.metrics.WritePrometheus(w)
}

// GetAdminUsername returns the admin username (for admin commands)
func (gw *Gateway) GetAdminUsername() string {
	// Configured admin username takes priority
	if gw.cfg.Admin.Username != "" {
		return gw.cfg.Admin.Username
	}
	// Default to first allowed Telegram user
	if len(gw.cfg.Channels.Telegram.AllowedUsers) > 0 {
		return gw.cfg.Channels.Telegram.AllowedUsers[0]
	}
	return ""
}

// GetSystemStats returns system statistics for admin commands
func (gw *Gateway) GetSystemStats() map[string]any {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]any{
		"uptime":          formatDuration(time.Since(gw.startTime)),
		"uptime_seconds":  int(time.Since(gw.startTime).Seconds()),
		"goroutines":      runtime.NumGoroutine(),
		"memory_alloc_mb": m.Alloc / 1024 / 1024,
		"memory_sys_mb":   m.Sys / 1024 / 1024,
		"sessions":        gw.sessions.Count(),
		"gc_cycles":       m.NumGC,
	}
}

// GetAllSessions returns all active sessions for admin commands
func (gw *Gateway) GetAllSessions() []*session.Session {
	return gw.sessions.GetRecent(1000)
}

// ReloadConfig triggers a config reload
func (gw *Gateway) ReloadConfig(ctx context.Context) error {
	gw.handleConfigReload(ctx)
	return nil
}

// ListSubAgents returns all sub-agents for command handler
func (gw *Gateway) ListSubAgents() []command.SubAgentInfo {
	if gw.subagentManager == nil {
		return nil
	}

	agents := gw.subagentManager.List()
	result := make([]command.SubAgentInfo, len(agents))

	for i, sa := range agents {
		status, label, task, resultText, errMsg := sa.GetInfo()
		result[i] = command.SubAgentInfo{
			ID:     sa.ID,
			Label:  label,
			Task:   task,
			Status: status,
			Result: resultText,
			Error:  errMsg,
		}
	}

	return result
}

// GetSubAgent returns a specific sub-agent for command handler
func (gw *Gateway) GetSubAgent(id string) (*command.SubAgentInfo, bool) {
	if gw.subagentManager == nil {
		return nil, false
	}

	sa, exists := gw.subagentManager.Get(id)
	if !exists {
		return nil, false
	}

	status, label, task, resultText, errMsg := sa.GetInfo()
	return &command.SubAgentInfo{
		ID:     sa.ID,
		Label:  label,
		Task:   task,
		Status: status,
		Result: resultText,
		Error:  errMsg,
	}, true
}

// CancelSubAgent cancels a running sub-agent
func (gw *Gateway) CancelSubAgent(id string) error {
	if gw.subagentManager == nil {
		return fmt.Errorf("sub-agent manager not available")
	}
	return gw.subagentManager.Cancel(id)
}
