package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/browser"
	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/command"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/heartbeat"
	"github.com/FeelPulse/feelpulse/internal/memory"
	"github.com/FeelPulse/feelpulse/internal/ratelimit"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/internal/store"
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
	browser        *browser.Browser
	toolRegistry   *tools.Registry
	startTime      time.Time
	lastMessageAt  time.Time
	cancelCtx      context.CancelFunc
	activeRequests sync.WaitGroup // tracks in-flight message processing
	shutdownCh     chan struct{}  // signals shutdown in progress
	mu             sync.RWMutex   // protects router, telegram, compactor during hot reload
}

func New(cfg *config.Config) *Gateway {
	sessions := session.NewStore()

	// Initialize SQLite session persistence
	dbPath := store.DefaultDBPath()
	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		log.Printf("⚠️  Failed to initialize session database: %v", err)
	} else {
		if err := sessions.SetPersister(sqliteStore); err != nil {
			log.Printf("⚠️  Failed to load persisted sessions: %v", err)
		}
	}

	// Initialize memory/workspace manager
	workspacePath := cfg.Workspace.Path
	if workspacePath == "" {
		workspacePath = memory.DefaultWorkspacePath()
	}
	memMgr := memory.NewManager(workspacePath)
	if err := memMgr.Load(); err != nil {
		log.Printf("⚠️  Failed to load workspace files: %v", err)
	} else if memMgr.Soul() != "" || memMgr.User() != "" || memMgr.Memory() != "" {
		log.Printf("📂 Workspace loaded from %s", workspacePath)
	}

	// Initialize rate limiter
	limiter := ratelimit.New(cfg.Agent.RateLimit)
	if cfg.Agent.RateLimit > 0 {
		log.Printf("⏱️  Rate limiting enabled: %d messages/minute", cfg.Agent.RateLimit)
	}

	// Initialize usage tracker
	usageTracker := usage.NewTracker()

	// Initialize tool registry with built-in tools
	toolRegistry := tools.NewRegistry()
	tools.RegisterBuiltins(toolRegistry)

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
		startTime:    time.Now(),
		shutdownCh:   make(chan struct{}),
	}
	gw.commands.SetUsageTracker(usageTracker)
	gw.setupRoutes()
	return gw
}

func (gw *Gateway) setupRoutes() {
	gw.mux.HandleFunc("/health", gw.handleHealth)
	gw.mux.HandleFunc("/hooks/", gw.handleHook)
	gw.mux.HandleFunc("/v1/chat/completions", gw.handleOpenAIChatCompletion)
	gw.mux.HandleFunc("/dashboard", gw.handleDashboard)
}

func (gw *Gateway) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	gw.cancelCtx = cancel

	// Initialize agent and telegram
	gw.initializeAgent(ctx)
	gw.initializeTelegram(ctx)

	// Initialize browser automation
	gw.initializeBrowser()

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

	log.Printf("🫀 Gateway listening on %s", addr)
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
		log.Printf("📱 Telegram bot stopped")
	}

	// Wait for active message processing to complete (with timeout)
	log.Printf("⏳ Waiting for active requests to complete...")
	done := make(chan struct{})
	go func() {
		gw.activeRequests.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("✅ All active requests completed")
	case <-time.After(30 * time.Second):
		log.Printf("⚠️  Timeout waiting for requests, forcing shutdown")
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
			log.Printf("💾 Sessions saved: %d", savedCount)
		}
	}

	// Close browser
	if gw.browser != nil {
		gw.browser.Close()
		log.Printf("🌐 Browser closed")
	}

	// Close database connection
	if gw.db != nil {
		if err := gw.db.Close(); err != nil {
			log.Printf("⚠️  Error closing database: %v", err)
		} else {
			log.Printf("💾 Database connection closed")
		}
	}

	// Cancel context and stop server
	cancel()
	gw.server.Close()

	log.Printf("👋 Shutdown complete")
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
			log.Printf("⚠️  Failed to save session %s: %v", sess.Key, err)
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
		log.Printf("⚠️  Agent not configured: %v", err)
		return
	}

	// Inject workspace files into system prompt
	router.SetSystemPromptBuilder(gw.memory.BuildSystemPrompt)

	// Wire up tool registry for agentic tool calling
	if gw.toolRegistry != nil {
		router.SetToolRegistry(gw.toolRegistry)
		toolCount := len(gw.toolRegistry.List())
		if toolCount > 0 {
			log.Printf("🔧 Tool registry attached with %d tools", toolCount)
		}
	}

	gw.mu.Lock()
	gw.router = router
	gw.mu.Unlock()

	log.Printf("🤖 Agent initialized: %s/%s", gw.cfg.Agent.Provider, gw.cfg.Agent.Model)

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
		log.Printf("📦 Context compaction enabled (threshold: %dk tokens)", maxTokens/1000)
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
		log.Printf("🔒 Telegram allowlist: %v", gw.cfg.Channels.Telegram.AllowedUsers)
	}

	if err := telegram.Start(ctx); err != nil {
		log.Printf("⚠️  Failed to start telegram: %v", err)
		return
	}

	gw.mu.Lock()
	gw.telegram = telegram
	gw.mu.Unlock()

	log.Printf("📱 Telegram streaming enabled for responsive UX")
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
				log.Printf("⚠️ Failed to send heartbeat to %d: %v", userID, err)
			} else {
				log.Printf("💓 Sent heartbeat to user %d", userID)
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
		log.Printf("⚠️  Browser tools disabled: %v", err)
		return
	}

	// Set screenshot callback to send to Telegram
	b.SetScreenshotCallback(func(path string) error {
		// This callback is called when a screenshot is taken
		// We'll handle sending in the message handler based on context
		log.Printf("📸 Screenshot saved: %s", path)
		return nil
	})

	gw.browser = b

	// Register browser tools with the tool registry
	if gw.toolRegistry != nil {
		gw.registerBrowserTools(b)
	}

	log.Printf("🌐 Browser automation enabled (headless=%v, stealth=%v)", cfg.Headless, cfg.Stealth)
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

	log.Printf("🔧 Registered %d browser tools", 6)
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
	log.Printf("👁️  Watching config for changes: %s", configPath)
}

// handleConfigReload handles config file changes
func (gw *Gateway) handleConfigReload(ctx context.Context) {
	newCfg, err := config.Load()
	if err != nil {
		log.Printf("⚠️  Failed to reload config: %v", err)
		return
	}

	log.Printf("🔄 Config reloaded")

	oldCfg := gw.cfg
	gw.cfg = newCfg

	// Reload workspace files
	if err := gw.memory.Load(); err != nil {
		log.Printf("⚠️  Failed to reload workspace files: %v", err)
	}

	// Check if agent needs reinitialization
	agentChanged := oldCfg.Agent.APIKey != newCfg.Agent.APIKey ||
		oldCfg.Agent.AuthToken != newCfg.Agent.AuthToken ||
		oldCfg.Agent.Model != newCfg.Agent.Model ||
		oldCfg.Agent.Provider != newCfg.Agent.Provider

	if agentChanged {
		log.Printf("🔄 Reinitializing agent...")
		gw.initializeAgent(ctx)
	}

	// Check if telegram needs reinitialization
	telegramChanged := oldCfg.Channels.Telegram.BotToken != newCfg.Channels.Telegram.BotToken ||
		oldCfg.Channels.Telegram.Enabled != newCfg.Channels.Telegram.Enabled

	if telegramChanged {
		log.Printf("🔄 Reinitializing Telegram...")
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
				log.Printf("🔒 Telegram allowlist updated: %v", newCfg.Channels.Telegram.AllowedUsers)
			}
		}
	}

	// Update commands handler with new config
	gw.commands = command.NewHandler(gw.sessions, newCfg)

	// Update rate limiter if changed
	if oldCfg.Agent.RateLimit != newCfg.Agent.RateLimit {
		gw.limiter = ratelimit.New(newCfg.Agent.RateLimit)
		if newCfg.Agent.RateLimit > 0 {
			log.Printf("⏱️  Rate limit updated: %d messages/minute", newCfg.Agent.RateLimit)
		} else {
			log.Printf("⏱️  Rate limiting disabled")
		}
	}
}

// handleMessageStreaming processes messages with streaming support for Telegram
func (gw *Gateway) handleMessageStreaming(msg *types.Message, onDelta func(delta string)) (reply *types.Message, err error) {
	// Track active request for graceful shutdown
	gw.activeRequests.Add(1)
	defer gw.activeRequests.Done()

	// Check if shutting down
	select {
	case <-gw.shutdownCh:
		return &types.Message{
			Text:    "⏳ Service is shutting down. Please try again in a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	default:
	}

	// Update last message timestamp
	gw.mu.Lock()
	gw.lastMessageAt = time.Now()
	gw.mu.Unlock()

	// Panic recovery - ensure we don't crash from unexpected panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ panic in handleMessageStreaming: %v", r)
			reply = &types.Message{
				Text:    "❌ An unexpected error occurred. Please try again.",
				Channel: msg.Channel,
				IsBot:   true,
			}
			err = nil // Return gracefully instead of crashing
		}
	}()

	// Register user for heartbeat (if enabled)
	if gw.heartbeat != nil {
		if uid, ok := gw.getUserIDInt64(msg); ok {
			gw.heartbeat.RegisterUser(msg.Channel, uid)
		}
	}

	// Check for slash commands first (exempt from rate limiting) - no streaming for commands
	if command.IsCommand(msg.Text) {
		return gw.commands.Handle(msg)
	}

	// Check rate limit
	userID := gw.getUserID(msg)
	if !gw.limiter.Allow(userID) {
		log.Printf("⏱️  Rate limited: %s", userID)
		return &types.Message{
			Text:    "⏱ Rate limit exceeded. Please wait a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Get router with read lock (safe during hot reload)
	gw.mu.RLock()
	router := gw.router
	compactor := gw.compactor
	gw.mu.RUnlock()

	if router == nil {
		return &types.Message{
			Text:    "🔧 AI agent not configured. Set your API key in config.yaml",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
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
			log.Printf("⚠️  Compaction failed: %v (using full history)", err)
		} else if len(compacted) < len(history) {
			log.Printf("📦 Compacted history: %d → %d messages", len(history), len(compacted))
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

	// Route to agent with streaming callback
	reply, err = router.ProcessWithHistoryStream(history, agent.StreamCallback(onDelta))
	if err != nil {
		log.Printf("❌ Agent error: %v", err)
		return &types.Message{
			Text:    "❌ Sorry, I encountered an error processing your message.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Add bot reply to session history (and persist)
	gw.sessions.AddMessageAndPersist(msg.Channel, userID, *reply)

	return reply, nil
}

// handleMessage processes incoming messages from channels
func (gw *Gateway) handleMessage(msg *types.Message) (reply *types.Message, err error) {
	// Track active request for graceful shutdown
	gw.activeRequests.Add(1)
	defer gw.activeRequests.Done()

	// Check if shutting down
	select {
	case <-gw.shutdownCh:
		return &types.Message{
			Text:    "⏳ Service is shutting down. Please try again in a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	default:
	}

	// Update last message timestamp
	gw.mu.Lock()
	gw.lastMessageAt = time.Now()
	gw.mu.Unlock()

	// Panic recovery - ensure we don't crash from unexpected panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ panic in handleMessage: %v", r)
			reply = &types.Message{
				Text:    "❌ An unexpected error occurred. Please try again.",
				Channel: msg.Channel,
				IsBot:   true,
			}
			err = nil // Return gracefully instead of crashing
		}
	}()

	// Register user for heartbeat (if enabled)
	if gw.heartbeat != nil {
		if uid, ok := gw.getUserIDInt64(msg); ok {
			gw.heartbeat.RegisterUser(msg.Channel, uid)
		}
	}

	// Check for slash commands first (exempt from rate limiting)
	if command.IsCommand(msg.Text) {
		return gw.commands.Handle(msg)
	}

	// Check rate limit
	userID := gw.getUserID(msg)
	if !gw.limiter.Allow(userID) {
		log.Printf("⏱️  Rate limited: %s", userID)
		return &types.Message{
			Text:    "⏱ Rate limit exceeded. Please wait a moment.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Get router with read lock (safe during hot reload)
	gw.mu.RLock()
	router := gw.router
	compactor := gw.compactor
	gw.mu.RUnlock()

	if router == nil {
		return &types.Message{
			Text:    "🔧 AI agent not configured. Set your API key in config.yaml",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Get session (userID already extracted above for rate limiting)
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
			log.Printf("⚠️  Compaction failed: %v (using full history)", err)
		} else if len(compacted) < len(history) {
			log.Printf("📦 Compacted history: %d → %d messages", len(history), len(compacted))
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

	// Route to agent with full history
	reply, err = router.ProcessWithHistory(history)
	if err != nil {
		log.Printf("❌ Agent error: %v", err)
		return &types.Message{
			Text:    "❌ Sorry, I encountered an error processing your message.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Add bot reply to session history (and persist)
	gw.sessions.AddMessageAndPersist(msg.Channel, userID, *reply)

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
	status := map[string]any{
		"ok":       true,
		"version":  "0.1.0",
		"channels": map[string]bool{},
	}

	gw.mu.RLock()
	channels := status["channels"].(map[string]bool)
	channels["telegram"] = gw.telegram != nil

	if gw.router != nil {
		status["agent"] = gw.router.Agent().Name()
	}
	gw.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (gw *Gateway) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if gw.cfg.Hooks.Token != "" {
		token := r.Header.Get("Authorization")
		expected := "Bearer " + gw.cfg.Hooks.Token
		if token != expected {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("📨 Hook received: %s", r.URL.Path)

	// TODO: Route to agent based on hook mappings

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
	})
}
