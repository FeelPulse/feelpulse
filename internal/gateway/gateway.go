package gateway

import (
	"context"
	"crypto/rand"
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
		Addr:    addr,
		Handler: gw.mux,
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

// initializeSubAgents sets up the sub-agent system with callbacks and tools
func (gw *Gateway) initializeSubAgents() {
	if gw.subagentManager == nil {
		return
	}

	// Create completion callback that injects results and sends notifications
	onComplete := func(agentID, label, result, parentSessionKey string, duration time.Duration, err error) {
		gw.handleSubAgentComplete(agentID, label, result, parentSessionKey, duration, err)
	}

	// Create a new manager with the callback (replace the placeholder one)
	gw.subagentManager = subagent.NewManager(onComplete)

	// Wire up persistence via adapter
	if gw.db != nil {
		adapter := &subAgentPersisterAdapter{db: gw.db}
		if err := gw.subagentManager.SetPersister(adapter); err != nil {
			gw.log.Warn("Failed to set up sub-agent persistence: %v", err)
		} else {
			gw.log.Info("🤖 Sub-agent persistence enabled (SQLite)")
		}
	}

	// Register sub-agent tools if we have a router
	gw.mu.RLock()
	router := gw.router
	gw.mu.RUnlock()

	if router != nil && gw.toolRegistry != nil {
		gw.registerSubAgentTools()
		gw.log.Info("🤖 Sub-agent tools registered (spawn_agent, agent_status, cancel_agent)")
	}
}

// registerSubAgentTools registers spawn_agent, agent_status, cancel_agent tools
func (gw *Gateway) registerSubAgentTools() {
	if gw.subagentManager == nil {
		return
	}

	// Create the chat function that sub-agents will use
	chatFunc := gw.createSubAgentChatFunc()

	// Create runner
	runner := subagent.NewSimpleRunner(chatFunc, subagent.DefaultMaxIterations)

	// Register tools with a factory that creates context per session
	// We need to register tools that will get the parent session key at call time
	gw.toolRegistry.Register(&tools.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a background sub-agent to work on a task autonomously. The agent will run independently and inject its result back into this conversation when done.",
		Parameters: []tools.Parameter{
			{Name: "task", Type: "string", Description: "The task for the sub-agent to complete", Required: true},
			{Name: "label", Type: "string", Description: "Short label to identify this sub-agent", Required: true},
			{Name: "system_prompt", Type: "string", Description: "Optional system prompt override for the sub-agent", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			task, _ := params["task"].(string)
			label, _ := params["label"].(string)
			systemPrompt, _ := params["system_prompt"].(string)

			if task == "" {
				return "", fmt.Errorf("task is required")
			}
			if label == "" {
				return "", fmt.Errorf("label is required")
			}

			// Get parent session key from context if available, otherwise use default
			parentKey := "unknown"
			if key, ok := ctx.Value("session_key").(string); ok {
				parentKey = key
			}

			agentID := gw.subagentManager.Spawn(task, label, systemPrompt, parentKey, runner, gw.toolRegistry)

			return fmt.Sprintf("✅ Sub-agent spawned!\n\nID: %s\nLabel: %s\nTask: %s\n\nThe agent is now running in the background. You'll be notified when it completes.",
				agentID, label, truncateForDisplay(task, 100)), nil
		},
	})

	gw.toolRegistry.Register(&tools.Tool{
		Name:        "agent_status",
		Description: "Check status of spawned sub-agents",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to check, or omit for all agents", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			agentID, _ := params["agent_id"].(string)

			if agentID != "" {
				agent, exists := gw.subagentManager.Get(agentID)
				if !exists {
					return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
				}
				return agent.GetStatus(), nil
			}

			agents := gw.subagentManager.List()
			if len(agents) == 0 {
				return "📭 No sub-agents have been spawned.", nil
			}

			var result string
			result = fmt.Sprintf("🤖 *Sub-agents* (%d)\n\n", len(agents))
			for _, sa := range agents {
				status, agentLabel, task, _, _ := sa.GetInfo()
				result += fmt.Sprintf("• `%s` (%s) — %s\n  Task: %s\n\n", sa.ID, agentLabel, formatSubAgentStatus(status), truncateForDisplay(task, 50))
			}
			return result, nil
		},
	})

	gw.toolRegistry.Register(&tools.Tool{
		Name:        "cancel_agent",
		Description: "Cancel a running sub-agent",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to cancel", Required: true},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			agentID, _ := params["agent_id"].(string)
			if agentID == "" {
				return "", fmt.Errorf("agent_id is required")
			}

			sa, exists := gw.subagentManager.Get(agentID)
			if !exists {
				return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
			}

			_, agentLabel, _, _, _ := sa.GetInfo()

			if err := gw.subagentManager.Cancel(agentID); err != nil {
				return fmt.Sprintf("❌ Failed to cancel: %v", err), nil
			}

			return fmt.Sprintf("🚫 Sub-agent canceled!\n\nID: %s\nLabel: %s", agentID, agentLabel), nil
		},
	})
}

// createSubAgentChatFunc creates a function that runs agent conversations for sub-agents
func (gw *Gateway) createSubAgentChatFunc() subagent.ChatWithToolsFunc {
	return func(messages []types.Message, systemPrompt string, toolRegistry *tools.Registry, maxIterations int) (*types.AgentResponse, error) {
		gw.mu.RLock()
		router := gw.router
		gw.mu.RUnlock()

		if router == nil {
			return nil, fmt.Errorf("agent not configured")
		}

		// Get the Anthropic client
		anthropicClient, ok := router.Agent().(*agent.AnthropicClient)
		if !ok {
			return nil, fmt.Errorf("sub-agents require Anthropic provider")
		}

		// Build Anthropic tools from registry
		var anthropicTools []agent.AnthropicTool
		if toolRegistry != nil {
			for _, tool := range toolRegistry.List() {
				schema := tool.ToAnthropicSchema()
				inputSchemaBytes, _ := json.Marshal(schema["input_schema"])
				anthropicTools = append(anthropicTools, agent.AnthropicTool{
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: inputSchemaBytes,
				})
			}
		}

		// Create tool executor
		// TODO: pass parent context for graceful cancellation
		executor := func(name string, input map[string]any) (string, error) {
			if toolRegistry == nil {
				return "", fmt.Errorf("no tools available")
			}
			tool := toolRegistry.Get(name)
			if tool == nil {
				return "", fmt.Errorf("unknown tool: %s", name)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			return tool.Handler(ctx, input)
		}

		// Run the agentic loop
		return anthropicClient.ChatWithTools(messages, systemPrompt, anthropicTools, executor, maxIterations, nil)
	}
}

// handleSubAgentComplete handles a sub-agent completion
func (gw *Gateway) handleSubAgentComplete(agentID, label, result, parentSessionKey string, duration time.Duration, err error) {
	gw.log.Info("🤖 Sub-agent '%s' (%s) completed in %s", label, agentID, formatDuration(duration))

	// Format duration for display
	durationStr := formatDuration(duration)

	// Build result message
	var message string
	if err != nil {
		message = fmt.Sprintf("🤖 Sub-agent **%s** failed after %s:\n\n❌ %v", label, durationStr, err)
	} else {
		// Truncate long results for notification
		preview := result
		if len(preview) > 500 {
			preview = preview[:497] + "..."
		}
		message = fmt.Sprintf("🤖 Sub-agent **%s** completed in %s:\n\n%s", label, durationStr, preview)
	}

	// Inject result into parent session
	if parentSessionKey != "" {
		gw.injectSubAgentResult(parentSessionKey, label, result, err)
	}

	// Send Telegram notification
	gw.sendSubAgentNotification(parentSessionKey, message)
}

// injectSubAgentResult adds the sub-agent result to the parent session history
func (gw *Gateway) injectSubAgentResult(sessionKey, label, result string, err error) {
	// Parse session key to get channel and userID
	parts := parseSessionKey(sessionKey)
	if len(parts) != 2 {
		gw.log.Warn("Invalid session key for sub-agent result injection: %s", sessionKey)
		return
	}

	channel := parts[0]
	userID := parts[1]

	// Build system message content
	var content string
	if err != nil {
		content = fmt.Sprintf("[Sub-agent \"%s\" failed]\nError: %v", label, err)
	} else {
		content = fmt.Sprintf("[Sub-agent \"%s\" completed]\nResult: %s", label, result)
	}

	// Create a system-style message
	msg := types.Message{
		Text:      content,
		Channel:   channel,
		From:      "system",
		IsBot:     false, // Mark as user so it appears in context
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"subagent_result": true,
			"subagent_label":  label,
		},
	}

	// Add to session
	gw.sessions.AddMessageAndPersist(channel, userID, msg)
	gw.log.Debug("📥 Injected sub-agent result into session %s", sessionKey)
}

// sendSubAgentNotification sends a notification via Telegram
func (gw *Gateway) sendSubAgentNotification(sessionKey, message string) {
	parts := parseSessionKey(sessionKey)
	if len(parts) != 2 {
		return
	}

	channel := parts[0]
	if channel != "telegram" {
		return
	}

	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		gw.log.Warn("Invalid user ID in session key: %s", parts[1])
		return
	}

	gw.mu.RLock()
	telegram := gw.telegram
	gw.mu.RUnlock()

	if telegram != nil {
		if err := telegram.SendMessage(userID, message, true); err != nil {
			gw.log.Warn("Failed to send sub-agent notification: %v", err)
		}
	}
}

// parseSessionKey splits "channel:userID" into parts
func parseSessionKey(key string) []string {
	return strings.SplitN(key, ":", 2)
}

// subAgentPersisterAdapter wraps SQLiteStore to implement subagent.Persister
type subAgentPersisterAdapter struct {
	db *store.SQLiteStore
}

// pinManager implements command.PinProvider using SQLite
type pinManager struct {
	db  *store.SQLiteStore
	log *logger.Logger
}

func newPinManager(db *store.SQLiteStore, log *logger.Logger) (*pinManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if err := db.EnsurePinsTable(); err != nil {
		return nil, fmt.Errorf("failed to create pins table: %w", err)
	}
	return &pinManager{db: db, log: log}, nil
}

func (pm *pinManager) AddPin(sessionKey, text string) (string, error) {
	// Generate unique ID
	b := make([]byte, 8)
	rand.Read(b)
	id := fmt.Sprintf("pin-%x", b)

	pin := &store.PinData{
		ID:         id,
		SessionKey: sessionKey,
		Text:       text,
		CreatedAt:  time.Now(),
	}

	if err := pm.db.SavePin(pin); err != nil {
		return "", err
	}

	pm.log.Debug("📌 Pin created: %s for session %s", id, sessionKey)
	return id, nil
}

func (pm *pinManager) ListPins(sessionKey string) []command.PinInfo {
	pins, err := pm.db.LoadPinsBySession(sessionKey)
	if err != nil {
		pm.log.Warn("Failed to load pins: %v", err)
		return nil
	}

	result := make([]command.PinInfo, len(pins))
	for i, p := range pins {
		result[i] = command.PinInfo{
			ID:        p.ID,
			Text:      p.Text,
			CreatedAt: p.CreatedAt,
		}
	}
	return result
}

func (pm *pinManager) RemovePin(id string) error {
	if err := pm.db.DeletePin(id); err != nil {
		return err
	}
	pm.log.Debug("📌 Pin deleted: %s", id)
	return nil
}

func (pm *pinManager) GetPins(sessionKey string) string {
	pins, err := pm.db.LoadPinsBySession(sessionKey)
	if err != nil || len(pins) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n[USER PINNED INFORMATION - Always consider this context]\n")
	for _, p := range pins {
		sb.WriteString(fmt.Sprintf("- %s\n", p.Text))
	}
	return sb.String()
}

func (a *subAgentPersisterAdapter) EnsureSubAgentsTable() error {
	return a.db.EnsureSubAgentsTable()
}

func (a *subAgentPersisterAdapter) SaveSubAgent(sa *subagent.SubAgentData) error {
	return a.db.SaveSubAgent(&store.SubAgentData{
		ID:               sa.ID,
		Label:            sa.Label,
		Task:             sa.Task,
		SystemPrompt:     sa.SystemPrompt,
		Status:           sa.Status,
		Result:           sa.Result,
		Error:            sa.Error,
		StartedAt:        sa.StartedAt,
		CompletedAt:      sa.CompletedAt,
		ParentSessionKey: sa.ParentSessionKey,
	})
}

func (a *subAgentPersisterAdapter) DeleteSubAgent(id string) error {
	return a.db.DeleteSubAgent(id)
}

func (a *subAgentPersisterAdapter) LoadAllSubAgents() ([]*subagent.SubAgentData, error) {
	dbAgents, err := a.db.LoadAllSubAgents()
	if err != nil {
		return nil, err
	}
	result := make([]*subagent.SubAgentData, len(dbAgents))
	for i, sa := range dbAgents {
		result[i] = &subagent.SubAgentData{
			ID:               sa.ID,
			Label:            sa.Label,
			Task:             sa.Task,
			SystemPrompt:     sa.SystemPrompt,
			Status:           sa.Status,
			Result:           sa.Result,
			Error:            sa.Error,
			StartedAt:        sa.StartedAt,
			CompletedAt:      sa.CompletedAt,
			ParentSessionKey: sa.ParentSessionKey,
		}
	}
	return result, nil
}

func (a *subAgentPersisterAdapter) LoadPendingSubAgents() ([]*subagent.SubAgentData, error) {
	dbAgents, err := a.db.LoadPendingSubAgents()
	if err != nil {
		return nil, err
	}
	result := make([]*subagent.SubAgentData, len(dbAgents))
	for i, sa := range dbAgents {
		result[i] = &subagent.SubAgentData{
			ID:               sa.ID,
			Label:            sa.Label,
			Task:             sa.Task,
			SystemPrompt:     sa.SystemPrompt,
			Status:           sa.Status,
			Result:           sa.Result,
			Error:            sa.Error,
			StartedAt:        sa.StartedAt,
			CompletedAt:      sa.CompletedAt,
			ParentSessionKey: sa.ParentSessionKey,
		}
	}
	return result, nil
}

// truncateForDisplay truncates a string for display
func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatSubAgentStatus returns emoji-formatted status
func formatSubAgentStatus(status string) string {
	switch status {
	case subagent.StatusPending:
		return "⏳ Pending"
	case subagent.StatusRunning:
		return "🔄 Running"
	case subagent.StatusDone:
		return "✅ Done"
	case subagent.StatusFailed:
		return "❌ Failed"
	case subagent.StatusCanceled:
		return "🚫 Canceled"
	default:
		return status
	}
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
	token := r.Header.Get("Authorization")
	if token != "Bearer "+gw.cfg.Hooks.Token {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
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
