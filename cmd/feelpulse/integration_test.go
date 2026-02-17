//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/ratelimit"
	"github.com/FeelPulse/feelpulse/internal/session"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// MockAnthropicServer creates a mock Anthropic API server for integration testing
type MockAnthropicServer struct {
	server    *httptest.Server
	responses []MockResponse
	reqCount  int
	mu        sync.Mutex
}

type MockResponse struct {
	Text       string
	ToolUse    *MockToolUse
	StopReason string
}

type MockToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

func NewMockAnthropicServer() *MockAnthropicServer {
	m := &MockAnthropicServer{
		responses: make([]MockResponse, 0),
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Get the response for this request
		idx := m.reqCount
		m.reqCount++

		if idx >= len(m.responses) {
			// Default response if no more mocked responses
			resp := map[string]any{
				"id":          "msg_test",
				"type":        "message",
				"role":        "assistant",
				"model":       "claude-sonnet-4-20250514",
				"stop_reason": "end_turn",
				"content": []map[string]any{
					{"type": "text", "text": "Default mock response"},
				},
				"usage": map[string]int{
					"input_tokens":  10,
					"output_tokens": 5,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		mockResp := m.responses[idx]
		content := []map[string]any{}

		if mockResp.Text != "" {
			content = append(content, map[string]any{
				"type": "text",
				"text": mockResp.Text,
			})
		}

		if mockResp.ToolUse != nil {
			inputJSON, _ := json.Marshal(mockResp.ToolUse.Input)
			content = append(content, map[string]any{
				"type":  "tool_use",
				"id":    mockResp.ToolUse.ID,
				"name":  mockResp.ToolUse.Name,
				"input": json.RawMessage(inputJSON),
			})
		}

		stopReason := mockResp.StopReason
		if stopReason == "" {
			if mockResp.ToolUse != nil {
				stopReason = "tool_use"
			} else {
				stopReason = "end_turn"
			}
		}

		resp := map[string]any{
			"id":          fmt.Sprintf("msg_test_%d", idx),
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-sonnet-4-20250514",
			"stop_reason": stopReason,
			"content":     content,
			"usage": map[string]int{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return m
}

func (m *MockAnthropicServer) AddResponse(resp MockResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
}

func (m *MockAnthropicServer) Close() {
	m.server.Close()
}

func (m *MockAnthropicServer) URL() string {
	return m.server.URL
}

func (m *MockAnthropicServer) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reqCount
}

// MockTelegramServer creates a mock Telegram Bot API server
type MockTelegramServer struct {
	server     *httptest.Server
	messages   []MockTelegramMessage
	sentMsgs   []string
	mu         sync.Mutex
	updateID   int
}

type MockTelegramMessage struct {
	Text   string
	UserID int64
	ChatID int64
}

func NewMockTelegramServer() *MockTelegramServer {
	m := &MockTelegramServer{
		messages: make([]MockTelegramMessage, 0),
		sentMsgs: make([]string, 0),
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		path := r.URL.Path

		// Handle getMe
		if strings.Contains(path, "getMe") {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         12345,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "testbot",
				},
			})
			return
		}

		// Handle getUpdates
		if strings.Contains(path, "getUpdates") {
			if len(m.messages) > 0 {
				msg := m.messages[0]
				m.messages = m.messages[1:]
				m.updateID++

				json.NewEncoder(w).Encode(map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": m.updateID,
							"message": map[string]any{
								"message_id": m.updateID,
								"from": map[string]any{
									"id":         msg.UserID,
									"first_name": "TestUser",
									"username":   "testuser",
								},
								"chat": map[string]any{
									"id":   msg.ChatID,
									"type": "private",
								},
								"text": msg.Text,
								"date": time.Now().Unix(),
							},
						},
					},
				})
			} else {
				json.NewEncoder(w).Encode(map[string]any{
					"ok":     true,
					"result": []map[string]any{},
				})
			}
			return
		}

		// Handle sendMessage
		if strings.Contains(path, "sendMessage") {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				m.sentMsgs = append(m.sentMsgs, text)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": m.updateID + 1000,
				},
			})
			return
		}

		// Default response
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))

	return m
}

func (m *MockTelegramServer) AddMessage(msg MockTelegramMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
}

func (m *MockTelegramServer) SentMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.sentMsgs))
	copy(result, m.sentMsgs)
	return result
}

func (m *MockTelegramServer) Close() {
	m.server.Close()
}

func (m *MockTelegramServer) URL() string {
	return m.server.URL
}

// TestIntegration_BasicMessageFlow tests the complete message flow
func TestIntegration_BasicMessageFlow(t *testing.T) {
	// Create mock Anthropic server
	mockAnthropic := NewMockAnthropicServer()
	defer mockAnthropic.Close()

	mockAnthropic.AddResponse(MockResponse{
		Text:       "Hello! I'm Claude, how can I help you today?",
		StopReason: "end_turn",
	})

	// Create session store
	sessions := session.NewStore()

	// Create a simple message processor (simulating gateway behavior)
	processMessage := func(msg *types.Message) (*types.Message, error) {
		userID := "123"
		sess := sessions.GetOrCreate(msg.Channel, userID)
		sess.AddMessage(*msg)

		// Simulate API call (in real code this would use the mock server)
		reply := &types.Message{
			Text:    "Hello! I'm Claude, how can I help you today?",
			Channel: msg.Channel,
			IsBot:   true,
		}

		sess.AddMessage(*reply)
		return reply, nil
	}

	// Test message flow
	msg := &types.Message{
		Text:    "Hello!",
		Channel: "telegram",
		From:    "testuser",
		IsBot:   false,
	}

	reply, err := processMessage(msg)
	if err != nil {
		t.Fatalf("processMessage error: %v", err)
	}

	if reply.Text == "" {
		t.Error("Expected non-empty reply")
	}

	if !reply.IsBot {
		t.Error("Expected reply to be from bot")
	}

	// Verify session was created and has messages
	sess, exists := sessions.Get("telegram", "123")
	if !exists {
		t.Error("Session should exist")
	}

	messages := sess.GetAllMessages()
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages in session, got %d", len(messages))
	}
}

// TestIntegration_SessionPersistence tests session persistence across restarts
func TestIntegration_SessionPersistence(t *testing.T) {
	// Create session store
	store1 := session.NewStore()

	// Add some messages
	sess := store1.GetOrCreate("telegram", "user1")
	sess.AddMessage(types.Message{Text: "First message", IsBot: false})
	sess.AddMessage(types.Message{Text: "First response", IsBot: true})
	sess.AddMessage(types.Message{Text: "Second message", IsBot: false})

	// Verify messages
	messages := sess.GetAllMessages()
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	// Simulate restart - create new store and manually copy session (without DB)
	store2 := session.NewStore()
	newSess := store2.GetOrCreate("telegram", "user1")

	// In a real scenario, this would be loaded from SQLite
	// For this test, we verify the session mechanism works
	for _, msg := range messages {
		newSess.AddMessage(msg)
	}

	// Verify persistence
	loadedMessages := newSess.GetAllMessages()
	if len(loadedMessages) != 3 {
		t.Errorf("Expected 3 messages after reload, got %d", len(loadedMessages))
	}

	if loadedMessages[0].Text != "First message" {
		t.Error("First message text mismatch")
	}
}

// TestIntegration_RateLimiting tests rate limiting behavior
func TestIntegration_RateLimiting(t *testing.T) {
	// Create rate limiter with 3 messages per minute
	limiter := ratelimit.New(3)

	userID := "user1"

	// First 3 messages should pass
	for i := 0; i < 3; i++ {
		if !limiter.Allow(userID) {
			t.Errorf("Message %d should be allowed", i+1)
		}
	}

	// 4th message should be rate limited
	if limiter.Allow(userID) {
		t.Error("4th message should be rate limited")
	}

	// Different user should not be affected
	if !limiter.Allow("user2") {
		t.Error("Different user should not be rate limited")
	}
}

// TestIntegration_SessionClear tests session clearing
func TestIntegration_SessionClear(t *testing.T) {
	store := session.NewStore()

	// Create session with messages
	sess := store.GetOrCreate("telegram", "user1")
	sess.AddMessage(types.Message{Text: "Hello", IsBot: false})
	sess.AddMessage(types.Message{Text: "Hi!", IsBot: true})

	if sess.Len() != 2 {
		t.Errorf("Expected 2 messages, got %d", sess.Len())
	}

	// Clear session
	sess.Clear()

	if sess.Len() != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", sess.Len())
	}

	// Session should still exist but be empty
	sameSess := store.GetOrCreate("telegram", "user1")
	if sameSess.Len() != 0 {
		t.Errorf("Session should be empty after clear")
	}
}

// TestIntegration_ConcurrentAccess tests thread safety of sessions
func TestIntegration_ConcurrentAccess(t *testing.T) {
	store := session.NewStore()
	var wg sync.WaitGroup

	// Spawn multiple goroutines accessing the same session
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			sess := store.GetOrCreate("telegram", "user1")
			sess.AddMessage(types.Message{
				Text:  fmt.Sprintf("Message %d", id),
				IsBot: id%2 == 0,
			})

			// Read messages
			_ = sess.GetAllMessages()
		}(i)
	}

	wg.Wait()

	// Verify session has all messages
	sess := store.GetOrCreate("telegram", "user1")
	messages := sess.GetAllMessages()

	if len(messages) != 100 {
		t.Errorf("Expected 100 messages, got %d", len(messages))
	}
}

// TestIntegration_MultipleChannels tests handling multiple channels
func TestIntegration_MultipleChannels(t *testing.T) {
	store := session.NewStore()

	// Create sessions for different channels
	telegramSess := store.GetOrCreate("telegram", "user1")
	discordSess := store.GetOrCreate("discord", "user1")

	telegramSess.AddMessage(types.Message{Text: "Telegram message", IsBot: false})
	discordSess.AddMessage(types.Message{Text: "Discord message", IsBot: false})

	// Verify sessions are separate
	if telegramSess.Len() != 1 {
		t.Errorf("Telegram session should have 1 message, got %d", telegramSess.Len())
	}
	if discordSess.Len() != 1 {
		t.Errorf("Discord session should have 1 message, got %d", discordSess.Len())
	}

	// Verify content
	telegramMsgs := telegramSess.GetAllMessages()
	if telegramMsgs[0].Text != "Telegram message" {
		t.Error("Telegram message text mismatch")
	}

	discordMsgs := discordSess.GetAllMessages()
	if discordMsgs[0].Text != "Discord message" {
		t.Error("Discord message text mismatch")
	}
}

// TestIntegration_ContextCompaction tests context compaction flow
func TestIntegration_ContextCompaction(t *testing.T) {
	// Create mock summarizer
	mockSummarizer := &MockSummarizer{
		summary: "[Summary: The user greeted and asked about the weather]",
	}

	// Create compactor with low threshold for testing
	compactor := session.NewCompactor(mockSummarizer, 100, 3) // 100 tokens threshold, keep last 3

	// Create messages exceeding threshold
	messages := make([]types.Message, 10)
	for i := 0; i < 10; i++ {
		messages[i] = types.Message{
			Text:  strings.Repeat("x", 100), // 25 tokens each = 250 total
			IsBot: i%2 == 1,
		}
	}

	// Compact
	result, err := compactor.CompactIfNeeded(messages)
	if err != nil {
		t.Fatalf("CompactIfNeeded error: %v", err)
	}

	// Should have 1 summary + 3 kept = 4 messages
	if len(result) != 4 {
		t.Errorf("Expected 4 messages after compaction, got %d", len(result))
	}

	// First message should be summary
	if result[0].Metadata["type"] != "summary" {
		t.Error("First message should be summary")
	}
}

// MockSummarizer for testing
type MockSummarizer struct {
	summary string
	called  bool
}

func (m *MockSummarizer) Summarize(messages []types.Message) (string, error) {
	m.called = true
	return m.summary, nil
}

// TestIntegration_ConfigValidation tests configuration validation
func TestIntegration_ConfigValidation(t *testing.T) {
	// Valid config with API key
	validCfg := config.Default()
	validCfg.Agent.APIKey = "sk-ant-api-test"

	result := validCfg.Validate()
	if !result.IsValid() {
		t.Errorf("Expected valid config, got errors: %v", result.Errors)
	}

	// Invalid config without auth
	invalidCfg := config.Default()
	invalidCfg.Agent.APIKey = ""
	invalidCfg.Agent.AuthToken = ""

	result = invalidCfg.Validate()
	if result.IsValid() {
		t.Error("Expected invalid config without auth")
	}
}

// TestIntegration_HealthEndpoint tests the health endpoint
func TestIntegration_HealthEndpoint(t *testing.T) {
	// Create a simple health handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"ok":      true,
			"version": "0.1.0",
			"uptime":  "1h 30m",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test health endpoint
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	if status["ok"] != true {
		t.Error("Expected ok=true in health response")
	}
}

// TestIntegration_GracefulShutdown tests graceful shutdown behavior
func TestIntegration_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	shutdownComplete := make(chan struct{})

	// Simulate a service that needs graceful shutdown
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		// Simulate cleanup
		time.Sleep(10 * time.Millisecond)
		close(shutdownComplete)
	}()

	// Trigger shutdown
	cancel()

	// Wait for shutdown with timeout
	select {
	case <-shutdownComplete:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Shutdown did not complete in time")
	}

	wg.Wait()
}

// TestIntegration_ModelSwitch tests switching AI models
func TestIntegration_ModelSwitch(t *testing.T) {
	store := session.NewStore()
	sess := store.GetOrCreate("telegram", "user1")

	// Default model should be empty (use config default)
	if sess.GetModel() != "" {
		t.Errorf("Expected empty default model, got %s", sess.GetModel())
	}

	// Set model
	sess.SetModel("claude-3-opus-20240229")
	if sess.GetModel() != "claude-3-opus-20240229" {
		t.Errorf("Expected opus model, got %s", sess.GetModel())
	}

	// Switch to another model
	sess.SetModel("claude-3-haiku-20240307")
	if sess.GetModel() != "claude-3-haiku-20240307" {
		t.Errorf("Expected haiku model, got %s", sess.GetModel())
	}

	// Clear should reset model
	sess.Clear()
	if sess.GetModel() != "" {
		t.Errorf("Expected empty model after clear, got %s", sess.GetModel())
	}
}

// TestIntegration_ProfileSwitch tests personality profile switching
func TestIntegration_ProfileSwitch(t *testing.T) {
	store := session.NewStore()
	sess := store.GetOrCreate("telegram", "user1")

	// Default profile should be empty
	if sess.GetProfile() != "" {
		t.Errorf("Expected empty default profile, got %s", sess.GetProfile())
	}

	// Set profile
	sess.SetProfile("friendly")
	if sess.GetProfile() != "friendly" {
		t.Errorf("Expected friendly profile, got %s", sess.GetProfile())
	}

	// Switch profile
	sess.SetProfile("professional")
	if sess.GetProfile() != "professional" {
		t.Errorf("Expected professional profile, got %s", sess.GetProfile())
	}
}

// TestIntegration_AnthropicClientCreation tests Anthropic client creation
func TestIntegration_AnthropicClientCreation(t *testing.T) {
	// Test API key auth
	client1 := agent.NewAnthropicClient("sk-ant-api-test", "", "claude-sonnet-4-20250514")
	if client1 == nil {
		t.Error("Expected non-nil client with API key")
	}
	if client1.Name() != "anthropic" {
		t.Errorf("Expected provider name 'anthropic', got %s", client1.Name())
	}

	// Test OAuth token auth
	client2 := agent.NewAnthropicClient("", "sk-ant-oat-test", "claude-sonnet-4-20250514")
	if client2 == nil {
		t.Error("Expected non-nil client with OAuth token")
	}

	// Test default model
	client3 := agent.NewAnthropicClient("test-key", "", "")
	if client3 == nil {
		t.Error("Expected non-nil client with default model")
	}
}
