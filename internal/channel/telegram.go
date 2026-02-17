package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// TelegramBot handles Telegram Bot API interactions
type TelegramBot struct {
	token   string
	baseURL string
	client  *http.Client
	offset  int64

	handler func(msg *types.Message) (*types.Message, error)
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// TelegramUpdate represents a Telegram update from getUpdates
type TelegramUpdate struct {
	UpdateID int64           `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage represents a Telegram message
type TelegramMessage struct {
	MessageID int64           `json:"message_id"`
	From      *TelegramUser   `json:"from,omitempty"`
	Chat      *TelegramChat   `json:"chat"`
	Date      int64           `json:"date"`
	Text      string          `json:"text,omitempty"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// TelegramResponse is the generic API response wrapper
type TelegramResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
}

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(token string) *TelegramBot {
	return &TelegramBot{
		token:   token,
		baseURL: "https://api.telegram.org/bot" + token,
		client: &http.Client{
			Timeout: 60 * time.Second, // Long polling timeout
		},
	}
}

// SetHandler sets the message handler function
func (t *TelegramBot) SetHandler(handler func(msg *types.Message) (*types.Message, error)) {
	t.handler = handler
}

// Start begins polling for updates
func (t *TelegramBot) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("bot is already running")
	}
	t.running = true
	ctx, t.cancel = context.WithCancel(ctx)
	t.mu.Unlock()

	log.Printf("📱 Telegram bot starting...")

	// Test connection by getting bot info
	me, err := t.GetMe()
	if err != nil {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
		return fmt.Errorf("failed to connect to Telegram: %w", err)
	}
	log.Printf("📱 Telegram bot connected: @%s", me.Username)

	// Start polling loop
	go t.pollLoop(ctx)

	return nil
}

// Stop stops the polling loop
func (t *TelegramBot) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	t.running = false
}

// GetMe returns bot info
func (t *TelegramBot) GetMe() (*TelegramUser, error) {
	resp, err := t.call("getMe", nil)
	if err != nil {
		return nil, err
	}

	var user TelegramUser
	if err := json.Unmarshal(resp.Result, &user); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &user, nil
}

// pollLoop continuously polls for updates
func (t *TelegramBot) pollLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("📱 Telegram bot stopped")
			return
		default:
			if err := t.poll(ctx); err != nil {
				log.Printf("❌ Telegram poll error: %v", err)
				time.Sleep(5 * time.Second) // Back off on error
			}
		}
	}
}

// poll fetches and processes updates
func (t *TelegramBot) poll(ctx context.Context) error {
	params := map[string]any{
		"offset":  t.offset,
		"timeout": 30, // Long polling
		"allowed_updates": []string{"message"},
	}

	resp, err := t.call("getUpdates", params)
	if err != nil {
		return err
	}

	var updates []TelegramUpdate
	if err := json.Unmarshal(resp.Result, &updates); err != nil {
		return fmt.Errorf("failed to parse updates: %w", err)
	}

	for _, update := range updates {
		// Update offset to acknowledge this update
		if update.UpdateID >= t.offset {
			t.offset = update.UpdateID + 1
		}

		if update.Message != nil && update.Message.Text != "" {
			t.handleMessage(ctx, update.Message)
		}
	}

	return nil
}

// handleMessage processes an incoming message
func (t *TelegramBot) handleMessage(ctx context.Context, tgMsg *TelegramMessage) {
	if t.handler == nil {
		return
	}

	// Convert to our Message type
	msg := &types.Message{
		ID:        strconv.FormatInt(tgMsg.MessageID, 10),
		Text:      tgMsg.Text,
		Channel:   "telegram",
		Timestamp: time.Unix(tgMsg.Date, 0),
		IsBot:     false,
		Metadata: map[string]any{
			"chat_id": tgMsg.Chat.ID,
		},
	}

	if tgMsg.From != nil {
		msg.From = tgMsg.From.Username
		if msg.From == "" {
			msg.From = tgMsg.From.FirstName
		}
		msg.Metadata["user_id"] = tgMsg.From.ID
	}

	log.Printf("📨 [%s] %s: %s", msg.Channel, msg.From, msg.Text)

	// Call handler
	reply, err := t.handler(msg)
	if err != nil {
		log.Printf("❌ Handler error: %v", err)
		return
	}

	if reply != nil && reply.Text != "" {
		if err := t.SendMessage(tgMsg.Chat.ID, reply.Text, true); err != nil {
			log.Printf("❌ Failed to send reply: %v", err)
		}
	}
}

// SendMessage sends a message to a chat
func (t *TelegramBot) SendMessage(chatID int64, text string, markdown bool) error {
	params := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}

	if markdown {
		params["parse_mode"] = "Markdown"
	}

	_, err := t.call("sendMessage", params)
	return err
}

// call makes an API call to Telegram
func (t *TelegramBot) call(method string, params map[string]any) (*TelegramResponse, error) {
	url := t.baseURL + "/" + method

	var body io.Reader
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tgResp TelegramResponse
	if err := json.Unmarshal(respBody, &tgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !tgResp.OK {
		return nil, fmt.Errorf("telegram API error: %s (code: %d)", tgResp.Description, tgResp.ErrorCode)
	}

	return &tgResp, nil
}
