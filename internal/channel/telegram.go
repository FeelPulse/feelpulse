package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
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

	handler         func(msg *types.Message) (*types.Message, error)
	callbackHandler func(chatID int64, userID int64, action, value string) (string, *InlineKeyboard, error)
	allowedUsers    []string // empty = allow all; non-empty = only these usernames
	mu              sync.Mutex
	running         bool
	cancel          context.CancelFunc
}

// TelegramUpdate represents a Telegram update from getUpdates
type TelegramUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *TelegramMessage `json:"message,omitempty"`
	CallbackQuery *CallbackQuery   `json:"callback_query,omitempty"`
}

// CallbackQuery represents a callback query from an inline keyboard button press
type CallbackQuery struct {
	ID      string           `json:"id"`
	From    *TelegramUser    `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data,omitempty"`
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

// SetCallbackHandler sets the callback query handler function
func (t *TelegramBot) SetCallbackHandler(handler func(chatID int64, userID int64, action, value string) (string, *InlineKeyboard, error)) {
	t.callbackHandler = handler
}

// SetAllowedUsers sets the allowlist of usernames
// Empty list means all users are allowed
func (t *TelegramBot) SetAllowedUsers(users []string) {
	t.allowedUsers = users
}

// IsUserAllowed checks if a username is in the allowlist
// Returns true if allowlist is empty (allow all) or if username is in the list
func (t *TelegramBot) IsUserAllowed(username string) bool {
	// Empty allowlist = allow everyone
	if len(t.allowedUsers) == 0 {
		return true
	}

	// Check if username is in allowlist
	for _, allowed := range t.allowedUsers {
		if allowed == username {
			return true
		}
	}
	return false
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

	// Register bot commands menu
	if err := t.SetMyCommands(); err != nil {
		log.Printf("⚠️ Failed to set bot commands: %v", err)
	} else {
		log.Printf("📋 Bot commands menu registered")
	}

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
		"allowed_updates": []string{"message", "callback_query"},
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

		if update.CallbackQuery != nil {
			t.handleCallbackQuery(ctx, update.CallbackQuery)
		}
	}

	return nil
}

// handleMessage processes an incoming message
func (t *TelegramBot) handleMessage(ctx context.Context, tgMsg *TelegramMessage) {
	if t.handler == nil {
		return
	}

	// Get username for allowlist check
	var username string
	if tgMsg.From != nil {
		username = tgMsg.From.Username
	}

	// Check allowlist before processing
	if !t.IsUserAllowed(username) {
		log.Printf("⛔ Blocked message from unauthorized user: %s", username)
		_ = t.SendMessage(tgMsg.Chat.ID, "⛔ You are not authorized to use this bot.", false)
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

	// Send typing indicator
	if err := t.SendTypingAction(tgMsg.Chat.ID); err != nil {
		log.Printf("⚠️ Failed to send typing action: %v", err)
	}

	// Call handler
	reply, err := t.handler(msg)
	if err != nil {
		log.Printf("❌ Handler error: %v", err)
		return
	}

	if reply != nil && reply.Text != "" {
		// Check if this is an export response (should send as file)
		if reply.Metadata != nil {
			if export, ok := reply.Metadata["export"].(bool); ok && export {
				filename, _ := reply.Metadata["filename"].(string)
				if filename == "" {
					filename = "export.txt"
				}
				content := []byte(reply.Text)
				if err := t.SendDocument(tgMsg.Chat.ID, filename, content, "📤 Conversation export"); err != nil {
					log.Printf("❌ Failed to send export file: %v", err)
					_ = t.SendMessage(tgMsg.Chat.ID, "❌ Failed to export conversation.", false)
				}
				return
			}
		}

		// Check if reply has a keyboard
		if reply.Keyboard != nil {
			if keyboard, ok := reply.Keyboard.(InlineKeyboard); ok {
				if err := t.SendMessageWithKeyboard(tgMsg.Chat.ID, reply.Text, keyboard, true); err != nil {
					log.Printf("❌ Failed to send reply with keyboard: %v", err)
				}
				return
			}
		}
		// No keyboard, send regular message
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

// SendTypingAction sends a "typing" indicator to the chat
func (t *TelegramBot) SendTypingAction(chatID int64) error {
	params := map[string]any{
		"chat_id": chatID,
		"action":  "typing",
	}

	_, err := t.call("sendChatAction", params)
	return err
}

// SendDocument sends a document/file to a chat
func (t *TelegramBot) SendDocument(chatID int64, filename string, content []byte, caption string) error {
	url := t.baseURL + "/sendDocument"

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add chat_id field
	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return fmt.Errorf("failed to write chat_id: %w", err)
	}

	// Add caption if provided
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return fmt.Errorf("failed to write caption: %w", err)
		}
	}

	// Add document file
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("failed to write document: %w", err)
	}

	// Close writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var tgResp TelegramResponse
	if err := json.Unmarshal(respBody, &tgResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !tgResp.OK {
		return fmt.Errorf("telegram API error: %s (code: %d)", tgResp.Description, tgResp.ErrorCode)
	}

	return nil
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

// SetMyCommands registers bot commands with Telegram for the "/" menu
func (t *TelegramBot) SetMyCommands() error {
	params := map[string]any{
		"commands": BotCommands(),
	}

	_, err := t.call("setMyCommands", params)
	return err
}

// handleCallbackQuery processes an inline keyboard button press
func (t *TelegramBot) handleCallbackQuery(ctx context.Context, query *CallbackQuery) {
	if t.callbackHandler == nil {
		// Answer with empty response to clear loading state
		_ = t.AnswerCallbackQuery(query.ID, "")
		return
	}

	var chatID int64
	if query.Message != nil && query.Message.Chat != nil {
		chatID = query.Message.Chat.ID
	}

	var userID int64
	if query.From != nil {
		userID = query.From.ID
	}

	action, value := ParseCallbackData(query.Data)
	log.Printf("📲 Callback: action=%s value=%s from=%d", action, value, userID)

	text, keyboard, err := t.callbackHandler(chatID, userID, action, value)
	if err != nil {
		log.Printf("❌ Callback handler error: %v", err)
		_ = t.AnswerCallbackQuery(query.ID, "Error processing request")
		return
	}

	// Answer the callback query to remove loading state
	_ = t.AnswerCallbackQuery(query.ID, "")

	// If handler returned text, edit the original message
	if text != "" && query.Message != nil {
		if err := t.EditMessageText(chatID, query.Message.MessageID, text, keyboard); err != nil {
			log.Printf("⚠️ Failed to edit message: %v", err)
			// Fallback to sending a new message
			if keyboard != nil {
				_ = t.SendMessageWithKeyboard(chatID, text, *keyboard, true)
			} else {
				_ = t.SendMessage(chatID, text, true)
			}
		}
	}
}

// AnswerCallbackQuery sends a response to a callback query
func (t *TelegramBot) AnswerCallbackQuery(queryID string, text string) error {
	params := map[string]any{
		"callback_query_id": queryID,
	}
	if text != "" {
		params["text"] = text
	}

	_, err := t.call("answerCallbackQuery", params)
	return err
}

// SendMessageWithKeyboard sends a message with an inline keyboard
func (t *TelegramBot) SendMessageWithKeyboard(chatID int64, text string, keyboard InlineKeyboard, markdown bool) error {
	params := map[string]any{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": keyboard,
	}

	if markdown {
		params["parse_mode"] = "Markdown"
	}

	_, err := t.call("sendMessage", params)
	return err
}

// EditMessageText edits an existing message
func (t *TelegramBot) EditMessageText(chatID int64, messageID int64, text string, keyboard *InlineKeyboard) error {
	params := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	if keyboard != nil {
		params["reply_markup"] = keyboard
	}

	_, err := t.call("editMessageText", params)
	return err
}
