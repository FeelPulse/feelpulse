package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

const (
	discordAPIBase = "https://discord.com/api/v10"
	discordGateway = "wss://gateway.discord.gg/?v=10&encoding=json"
)

// Message is an alias for types.Message to avoid import issues in tests
type Message = types.Message

// DiscordBot handles Discord Bot API interactions
type DiscordBot struct {
	token   string
	client  *http.Client
	handler func(msg *types.Message) (*types.Message, error)
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	botID   string
}

// DiscordUser represents a Discord user
type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Bot           bool   `json:"bot"`
}

// DiscordMessage represents a Discord message
type DiscordMessage struct {
	ID        string       `json:"id"`
	Content   string       `json:"content"`
	ChannelID string       `json:"channel_id"`
	GuildID   string       `json:"guild_id,omitempty"`
	Author    *DiscordUser `json:"author"`
	Timestamp string       `json:"timestamp"`
}

// DiscordGatewayPayload represents a Discord Gateway message
type DiscordGatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int            `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

// DiscordHelloEvent represents the Hello event from Gateway
type DiscordHelloEvent struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// DiscordIdentify represents the Identify payload
type DiscordIdentify struct {
	Token   string                 `json:"token"`
	Intents int                    `json:"intents"`
	Properties map[string]string   `json:"properties"`
}

// DiscordReadyEvent represents the Ready event
type DiscordReadyEvent struct {
	User *DiscordUser `json:"user"`
}

// DiscordMessageCreate represents the MESSAGE_CREATE event
type DiscordMessageCreate struct {
	DiscordMessage
}

// NewDiscordBot creates a new Discord bot instance
func NewDiscordBot(token string) *DiscordBot {
	return &DiscordBot{
		token: token,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetHandler sets the message handler function
func (d *DiscordBot) SetHandler(handler func(msg *types.Message) (*types.Message, error)) {
	d.handler = handler
}

// Start begins the Discord bot connection
func (d *DiscordBot) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("bot is already running")
	}
	d.running = true
	ctx, d.cancel = context.WithCancel(ctx)
	d.mu.Unlock()

	log.Printf("💬 Discord bot starting...")

	// Get bot user info
	user, err := d.GetMe()
	if err != nil {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
		return fmt.Errorf("failed to connect to Discord: %w", err)
	}
	d.botID = user.ID
	log.Printf("💬 Discord bot connected: %s#%s", user.Username, user.Discriminator)

	// For simplicity, we'll use HTTP polling instead of WebSocket Gateway
	// In production, you'd want to use the Gateway WebSocket connection
	go d.pollMessages(ctx)

	return nil
}

// Stop stops the bot
func (d *DiscordBot) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cancel != nil {
		d.cancel()
	}
	d.running = false
}

// GetMe returns the bot user info
func (d *DiscordBot) GetMe() (*DiscordUser, error) {
	resp, err := d.apiCall("GET", "/users/@me", nil)
	if err != nil {
		return nil, err
	}

	var user DiscordUser
	if err := json.Unmarshal(resp, &user); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &user, nil
}

// pollMessages polls for new messages (simplified approach)
// In production, use Discord Gateway WebSocket
func (d *DiscordBot) pollMessages(ctx context.Context) {
	// Note: Discord doesn't have a direct polling API like Telegram
	// This is a placeholder - in production, you'd use:
	// 1. Discord Gateway (WebSocket) - recommended
	// 2. Message webhooks
	// 3. Slash commands
	
	log.Printf("💬 Discord message handling ready (webhook/slash command mode)")
	
	<-ctx.Done()
	log.Printf("💬 Discord bot stopped")
}

// handleMessage processes an incoming Discord message
func (d *DiscordBot) handleMessage(ctx context.Context, dm *DiscordMessage) {
	if d.handler == nil {
		return
	}

	// Ignore bot messages
	if dm.Author != nil && dm.Author.Bot {
		return
	}

	// Ignore our own messages
	if dm.Author != nil && dm.Author.ID == d.botID {
		return
	}

	msg := discordToMessage(dm)
	log.Printf("📨 [%s] %s: %s", msg.Channel, msg.From, msg.Text)

	// Send typing indicator
	d.SendTypingAction(dm.ChannelID)

	// Call handler
	reply, err := d.handler(msg)
	if err != nil {
		log.Printf("❌ Handler error: %v", err)
		return
	}

	if reply != nil && reply.Text != "" {
		if err := d.SendMessage(dm.ChannelID, reply.Text); err != nil {
			log.Printf("❌ Failed to send reply: %v", err)
		}
	}
}

// discordToMessage converts a Discord message to internal Message type
func discordToMessage(dm *DiscordMessage) *types.Message {
	msg := &types.Message{
		ID:      dm.ID,
		Text:    dm.Content,
		Channel: "discord",
		IsBot:   false,
		Metadata: map[string]any{
			"channel_id": dm.ChannelID,
		},
	}

	if dm.GuildID != "" {
		msg.Metadata["guild_id"] = dm.GuildID
	}

	if dm.Author != nil {
		msg.From = dm.Author.Username
		msg.Metadata["user_id"] = dm.Author.ID
		msg.IsBot = dm.Author.Bot
	}

	if dm.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, dm.Timestamp); err == nil {
			msg.Timestamp = t
		}
	}

	return msg
}

// SendMessage sends a message to a channel
func (d *DiscordBot) SendMessage(channelID, content string) error {
	body := map[string]any{
		"content": content,
	}

	_, err := d.apiCall("POST", "/channels/"+channelID+"/messages", body)
	return err
}

// SendTypingAction sends a typing indicator
func (d *DiscordBot) SendTypingAction(channelID string) error {
	_, err := d.apiCall("POST", "/channels/"+channelID+"/typing", nil)
	return err
}

// apiCall makes an API call to Discord
func (d *DiscordBot) apiCall(method, path string, body any) ([]byte, error) {
	url := discordAPIBase + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+d.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discord API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// HandleWebhook processes an incoming webhook from Discord
// This is for use with Discord Interactions/Slash Commands
func (d *DiscordBot) HandleWebhook(dm *DiscordMessage) (*types.Message, error) {
	if d.handler == nil {
		return nil, fmt.Errorf("no handler configured")
	}

	msg := discordToMessage(dm)
	return d.handler(msg)
}
