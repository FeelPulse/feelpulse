package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/FeelPulse/feelpulse/internal/config"
)

type Gateway struct {
	cfg    *config.Config
	mux    *http.ServeMux
	server *http.Server
}

func New(cfg *config.Config) *Gateway {
	gw := &Gateway{
		cfg: cfg,
		mux: http.NewServeMux(),
	}
	gw.setupRoutes()
	return gw
}

func (gw *Gateway) setupRoutes() {
	gw.mux.HandleFunc("/health", gw.handleHealth)
	gw.mux.HandleFunc("/hooks/", gw.handleHook)
}

func (gw *Gateway) Start() error {
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
		gw.server.Close()
	}()

	log.Printf("🫀 Gateway listening on %s", addr)
	return gw.server.ListenAndServe()
}

func (gw *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"version": "0.1.0",
	})
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
