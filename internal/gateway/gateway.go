package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/channel"
	"github.com/FeelPulse/feelpulse/internal/command"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

type Gateway struct {
	cfg      *config.Config
	mux      *http.ServeMux
	server   *http.Server
	telegram *channel.TelegramBot
	router   *agent.Router
	sessions *session.Store
	commands *command.Handler
}

func New(cfg *config.Config) *Gateway {
	sessions := session.NewStore()
	gw := &Gateway{
		cfg:      cfg,
		mux:      http.NewServeMux(),
		sessions: sessions,
		commands: command.NewHandler(sessions, cfg),
	}
	gw.setupRoutes()
	return gw
}

func (gw *Gateway) setupRoutes() {
	gw.mux.HandleFunc("/health", gw.handleHealth)
	gw.mux.HandleFunc("/hooks/", gw.handleHook)
}

func (gw *Gateway) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize agent router if configured
	if gw.cfg.Agent.APIKey != "" || gw.cfg.Agent.AuthToken != "" {
		router, err := agent.NewRouter(gw.cfg)
		if err != nil {
			log.Printf("⚠️  Agent not configured: %v", err)
		} else {
			gw.router = router
			log.Printf("🤖 Agent initialized: %s/%s", gw.cfg.Agent.Provider, gw.cfg.Agent.Model)
		}
	}

	// Initialize Telegram if configured
	if gw.cfg.Channels.Telegram.Enabled && gw.cfg.Channels.Telegram.BotToken != "" {
		gw.telegram = channel.NewTelegramBot(gw.cfg.Channels.Telegram.BotToken)
		gw.telegram.SetHandler(gw.handleMessage)

		if err := gw.telegram.Start(ctx); err != nil {
			return fmt.Errorf("failed to start telegram: %w", err)
		}
	}

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
		fmt.Println("\n👋 Shutting down...")
		if gw.telegram != nil {
			gw.telegram.Stop()
		}
		cancel()
		gw.server.Close()
	}()

	log.Printf("🫀 Gateway listening on %s", addr)
	return gw.server.ListenAndServe()
}

// handleMessage processes incoming messages from channels
func (gw *Gateway) handleMessage(msg *types.Message) (*types.Message, error) {
	// Check for slash commands first
	if command.IsCommand(msg.Text) {
		return gw.commands.Handle(msg)
	}

	if gw.router == nil {
		return &types.Message{
			Text:    "🔧 AI agent not configured. Set your API key in config.yaml",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Get user ID for session key
	userID := gw.getUserID(msg)
	sess := gw.sessions.GetOrCreate(msg.Channel, userID)

	// Add incoming message to session history
	sess.AddMessage(*msg)

	// Get conversation history for agent
	history := sess.GetAllMessages()

	// Route to agent with full history
	reply, err := gw.router.ProcessWithHistory(history)
	if err != nil {
		log.Printf("❌ Agent error: %v", err)
		return &types.Message{
			Text:    "❌ Sorry, I encountered an error processing your message.",
			Channel: msg.Channel,
			IsBot:   true,
		}, nil
	}

	// Add bot reply to session history
	sess.AddMessage(*reply)

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

func (gw *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"ok":       true,
		"version":  "0.1.0",
		"channels": map[string]bool{},
	}

	channels := status["channels"].(map[string]bool)
	channels["telegram"] = gw.telegram != nil
	
	if gw.router != nil {
		status["agent"] = gw.router.Agent().Name()
	}

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
