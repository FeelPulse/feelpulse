package channel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// StreamingHandler is called to process a message with streaming support.
// It should call onDelta for each text chunk as it arrives.
// Returns the final response text and error.
type StreamingHandler func(msg *types.Message, onDelta func(delta string)) (*types.Message, error)

// TelegramBot handles Telegram Bot API interactions
type TelegramBot struct {
	token   string
	baseURL string
	client  *http.Client
	offset  int64

	handler          func(msg *types.Message) (*types.Message, error)
	streamingHandler StreamingHandler
	callbackHandler  func(chatID int64, userID int64, action, value string) (string, *InlineKeyboard, error)
	allowedUsers     []string // empty = allow all; non-empty = only these usernames
	mu               sync.Mutex
	running          bool
	cancel           context.CancelFunc
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
	Caption   string          `json:"caption,omitempty"`
	Photo     []TelegramPhoto `json:"photo,omitempty"` // Array of PhotoSize, largest last
}

// TelegramPhoto represents a photo size in Telegram
type TelegramPhoto struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

// TelegramFile represents file info from getFile
type TelegramFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int    `json:"file_size,omitempty"`
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

// SetStreamingHandler sets the streaming message handler function
// When set, this handler is used instead of the regular handler for streaming responses.
func (t *TelegramBot) SetStreamingHandler(handler StreamingHandler) {
	t.streamingHandler = handler
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

		if update.Message != nil {
			// Handle text messages
			if update.Message.Text != "" {
				t.handleMessage(ctx, update.Message)
			}
			// Handle photo messages
			if len(update.Message.Photo) > 0 {
				t.handlePhotoMessage(ctx, update.Message)
			}
		}

		if update.CallbackQuery != nil {
			t.handleCallbackQuery(ctx, update.CallbackQuery)
		}
	}

	return nil
}

// handleMessage processes an incoming message
func (t *TelegramBot) handleMessage(ctx context.Context, tgMsg *TelegramMessage) {
	if t.handler == nil && t.streamingHandler == nil {
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

	var reply *types.Message
	var err error

	// Use streaming handler if available
	if t.streamingHandler != nil {
		reply, err = t.handleMessageWithStreaming(ctx, tgMsg.Chat.ID, msg)
	} else {
		// Call regular handler
		reply, err = t.handler(msg)
	}

	if err != nil {
		log.Printf("❌ Handler error: %v", err)
		return
	}

	// For streaming, the message was already sent/updated
	if t.streamingHandler != nil {
		return
	}

	if reply != nil && reply.Text != "" {
		t.sendReply(tgMsg.Chat.ID, reply)
	}
}

// handlePhotoMessage processes an incoming photo message
func (t *TelegramBot) handlePhotoMessage(ctx context.Context, tgMsg *TelegramMessage) {
	if t.handler == nil && t.streamingHandler == nil {
		return
	}

	// Get username for allowlist check
	var username string
	if tgMsg.From != nil {
		username = tgMsg.From.Username
	}

	// Check allowlist before processing
	if !t.IsUserAllowed(username) {
		log.Printf("⛔ Blocked photo from unauthorized user: %s", username)
		_ = t.SendMessage(tgMsg.Chat.ID, "⛔ You are not authorized to use this bot.", false)
		return
	}

	// Get the largest photo (last in array)
	if len(tgMsg.Photo) == 0 {
		return
	}
	photo := tgMsg.Photo[len(tgMsg.Photo)-1]

	// Check file size (max 5MB for safety)
	if photo.FileSize > 5*1024*1024 {
		log.Printf("⚠️ Photo too large: %d bytes", photo.FileSize)
		_ = t.SendMessage(tgMsg.Chat.ID, "⚠️ Photo is too large. Please send a smaller image (max 5MB).", false)
		return
	}

	// Get file info
	fileInfo, err := t.GetFile(photo.FileID)
	if err != nil {
		log.Printf("❌ Failed to get file info: %v", err)
		_ = t.SendMessage(tgMsg.Chat.ID, "❌ Failed to process photo.", false)
		return
	}

	// Download the file
	imageData, err := t.DownloadFile(fileInfo.FilePath)
	if err != nil {
		log.Printf("❌ Failed to download photo: %v", err)
		_ = t.SendMessage(tgMsg.Chat.ID, "❌ Failed to download photo.", false)
		return
	}

	// Determine media type from file path
	mediaType := "image/jpeg"
	if strings.HasSuffix(fileInfo.FilePath, ".png") {
		mediaType = "image/png"
	} else if strings.HasSuffix(fileInfo.FilePath, ".gif") {
		mediaType = "image/gif"
	} else if strings.HasSuffix(fileInfo.FilePath, ".webp") {
		mediaType = "image/webp"
	}

	// Encode to base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// Create message with image data
	text := tgMsg.Caption
	if text == "" {
		text = "What do you see in this image?"
	}

	msg := &types.Message{
		ID:        strconv.FormatInt(tgMsg.MessageID, 10),
		Text:      text,
		Channel:   "telegram",
		Timestamp: time.Unix(tgMsg.Date, 0),
		IsBot:     false,
		Metadata: map[string]any{
			"chat_id": tgMsg.Chat.ID,
			"image": map[string]string{
				"data":       imageBase64,
				"media_type": mediaType,
			},
		},
	}

	if tgMsg.From != nil {
		msg.From = tgMsg.From.Username
		if msg.From == "" {
			msg.From = tgMsg.From.FirstName
		}
		msg.Metadata["user_id"] = tgMsg.From.ID
	}

	log.Printf("📷 [%s] %s: [Photo] %s", msg.Channel, msg.From, text)

	// Send typing indicator
	if err := t.SendTypingAction(tgMsg.Chat.ID); err != nil {
		log.Printf("⚠️ Failed to send typing action: %v", err)
	}

	var reply *types.Message

	// Use streaming handler if available
	if t.streamingHandler != nil {
		reply, err = t.handleMessageWithStreaming(ctx, tgMsg.Chat.ID, msg)
	} else {
		// Call regular handler
		reply, err = t.handler(msg)
	}

	if err != nil {
		log.Printf("❌ Handler error: %v", err)
		return
	}

	// For streaming, the message was already sent/updated
	if t.streamingHandler != nil {
		return
	}

	if reply != nil && reply.Text != "" {
		t.sendReply(tgMsg.Chat.ID, reply)
	}
}

// handleMessageWithStreaming processes a message using streaming with message editing
func (t *TelegramBot) handleMessageWithStreaming(ctx context.Context, chatID int64, msg *types.Message) (*types.Message, error) {
	// Send initial "thinking..." message
	thinkingMsgID, err := t.SendMessageAndGetID(chatID, "💭 _Thinking..._", true)
	if err != nil {
		log.Printf("⚠️ Failed to send thinking message: %v", err)
		// Fall back to non-streaming
		if t.handler != nil {
			return t.handler(msg)
		}
		return nil, err
	}

	// Track accumulated text and last update time
	var accumulated strings.Builder
	var lastUpdate time.Time
	var mu sync.Mutex
	updateInterval := 500 * time.Millisecond

	// Create delta handler that updates the message periodically
	onDelta := func(delta string) {
		mu.Lock()
		accumulated.WriteString(delta)
		currentText := accumulated.String()
		shouldUpdate := time.Since(lastUpdate) >= updateInterval
		mu.Unlock()

		if shouldUpdate && currentText != "" {
			// Truncate for Telegram's 4096 char limit
			displayText := currentText
			if len(displayText) > 4000 {
				displayText = displayText[:4000] + "..."
			}
			if err := t.EditMessageText(chatID, thinkingMsgID, displayText, nil); err != nil {
				log.Printf("⚠️ Failed to update streaming message: %v", err)
			}
			mu.Lock()
			lastUpdate = time.Now()
			mu.Unlock()
		}
	}

	// Call streaming handler
	reply, err := t.streamingHandler(msg, onDelta)
	if err != nil {
		// Update message with error
		_ = t.EditMessageText(chatID, thinkingMsgID, "❌ Sorry, I encountered an error processing your message.", nil)
		return nil, err
	}

	// Final update with complete response
	if reply != nil && reply.Text != "" {
		parts := SplitLongMessage(reply.Text, SafeMessageLength)
		
		// First part: edit the thinking message
		if err := t.EditMessageText(chatID, thinkingMsgID, parts[0], nil); err != nil {
			log.Printf("⚠️ Failed to send final message update: %v", err)
			// Try sending as new message
			_ = t.SendMessage(chatID, parts[0], true)
		}
		
		// Additional parts: send as new messages
		for i := 1; i < len(parts); i++ {
			if err := t.SendMessage(chatID, parts[i], true); err != nil {
				log.Printf("❌ Failed to send continuation message: %v", err)
			}
		}
	}

	return reply, nil
}

// sendReply sends a reply message handling special cases (export, keyboard)
func (t *TelegramBot) sendReply(chatID int64, reply *types.Message) {
	// Check if this is an export response (should send as file)
	if reply.Metadata != nil {
		if export, ok := reply.Metadata["export"].(bool); ok && export {
			filename, _ := reply.Metadata["filename"].(string)
			if filename == "" {
				filename = "export.txt"
			}
			content := []byte(reply.Text)
			if err := t.SendDocument(chatID, filename, content, "📤 Conversation export"); err != nil {
				log.Printf("❌ Failed to send export file: %v", err)
				_ = t.SendMessage(chatID, "❌ Failed to export conversation.", false)
			}
			return
		}

		// Check if this is a browse response with screenshot
		if screenshotPath, ok := reply.Metadata["screenshot_path"].(string); ok && screenshotPath != "" {
			caption, _ := reply.Metadata["screenshot_caption"].(string)
			// Send text first
			if reply.Text != "" {
				parts := SplitLongMessage(reply.Text, SafeMessageLength)
				for _, part := range parts {
					if err := t.SendMessage(chatID, part, true); err != nil {
						log.Printf("❌ Failed to send reply: %v", err)
					}
				}
			}
			// Then send screenshot as photo
			if err := t.SendPhoto(chatID, screenshotPath, caption); err != nil {
				log.Printf("⚠️ Failed to send screenshot: %v (continuing without photo)", err)
			} else {
				log.Printf("📸 Screenshot sent: %s", screenshotPath)
			}
			return
		}
	}

	// Check if reply has a keyboard
	if reply.Keyboard != nil {
		if keyboard, ok := reply.Keyboard.(InlineKeyboard); ok {
			// For messages with keyboards, send first part with keyboard
			parts := SplitLongMessage(reply.Text, SafeMessageLength)
			if len(parts) == 1 {
				if err := t.SendMessageWithKeyboard(chatID, reply.Text, keyboard, true); err != nil {
					log.Printf("❌ Failed to send reply with keyboard: %v", err)
				}
			} else {
				// Send all parts, attach keyboard to last one
				for i, part := range parts {
					if i == len(parts)-1 {
						if err := t.SendMessageWithKeyboard(chatID, part, keyboard, true); err != nil {
							log.Printf("❌ Failed to send reply with keyboard: %v", err)
						}
					} else {
						if err := t.SendMessage(chatID, part, true); err != nil {
							log.Printf("❌ Failed to send reply part: %v", err)
						}
					}
				}
			}
			return
		}
	}
	
	// No keyboard - split long messages
	parts := SplitLongMessage(reply.Text, SafeMessageLength)
	for _, part := range parts {
		if err := t.SendMessage(chatID, part, true); err != nil {
			log.Printf("❌ Failed to send reply: %v", err)
		}
	}
}

// SendMessage sends a message to a chat
func (t *TelegramBot) SendMessage(chatID int64, text string, markdown bool) error {
	_, err := t.SendMessageAndGetID(chatID, text, markdown)
	return err
}

// SendMessageAndGetID sends a message and returns the message ID
func (t *TelegramBot) SendMessageAndGetID(chatID int64, text string, markdown bool) (int64, error) {
	params := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}

	if markdown {
		params["parse_mode"] = "Markdown"
	}

	resp, err := t.call("sendMessage", params)
	if err != nil {
		return 0, err
	}

	// Parse the message to get its ID
	var sentMsg TelegramMessage
	if err := json.Unmarshal(resp.Result, &sentMsg); err != nil {
		return 0, fmt.Errorf("failed to parse sent message: %w", err)
	}

	return sentMsg.MessageID, nil
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

// SendPhoto sends a photo to a chat
func (t *TelegramBot) SendPhoto(chatID int64, path string, caption string) error {
	url := t.baseURL + "/sendPhoto"

	// Open the file
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open photo: %w", err)
	}
	defer file.Close()

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

	// Add photo file
	part, err := writer.CreateFormFile("photo", filepath.Base(path))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy photo: %w", err)
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

// GetFile retrieves file info from Telegram
func (t *TelegramBot) GetFile(fileID string) (*TelegramFile, error) {
	params := map[string]any{
		"file_id": fileID,
	}

	resp, err := t.call("getFile", params)
	if err != nil {
		return nil, err
	}

	var file TelegramFile
	if err := json.Unmarshal(resp.Result, &file); err != nil {
		return nil, fmt.Errorf("failed to parse file info: %w", err)
	}

	return &file, nil
}

// DownloadFile downloads a file from Telegram and returns its contents
func (t *TelegramBot) DownloadFile(filePath string) ([]byte, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", t.token, filePath)

	resp, err := t.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	return data, nil
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
