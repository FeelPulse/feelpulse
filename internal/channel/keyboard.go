package channel

import "strings"

// BotCommand represents a Telegram bot command for setMyCommands
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// InlineButton is a simplified button definition
type InlineButton struct {
	Text         string
	CallbackData string
}

// InlineKeyboardButton is the Telegram API inline keyboard button
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboard is the Telegram API inline keyboard markup
type InlineKeyboard struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// BotCommands returns the list of bot commands for the menu
func BotCommands() []BotCommand {
	return []BotCommand{
		{Command: "new", Description: "Start a new conversation"},
		{Command: "history", Description: "Show recent messages"},
		{Command: "model", Description: "Show or switch AI model"},
		{Command: "skills", Description: "List loaded AI tools"},
		{Command: "usage", Description: "Show token usage stats"},
		{Command: "reminders", Description: "List active reminders"},
		{Command: "help", Description: "Show all commands"},
	}
}

// BuildInlineKeyboard creates a single-row inline keyboard
func BuildInlineKeyboard(buttons []InlineButton) InlineKeyboard {
	if len(buttons) == 0 {
		return InlineKeyboard{InlineKeyboard: [][]InlineKeyboardButton{}}
	}
	
	row := make([]InlineKeyboardButton, len(buttons))
	for i, btn := range buttons {
		row[i] = InlineKeyboardButton{
			Text:         btn.Text,
			CallbackData: btn.CallbackData,
		}
	}
	
	return InlineKeyboard{InlineKeyboard: [][]InlineKeyboardButton{row}}
}

// BuildInlineKeyboardRows creates a multi-row inline keyboard
func BuildInlineKeyboardRows(rows [][]InlineButton) InlineKeyboard {
	result := make([][]InlineKeyboardButton, len(rows))
	
	for i, row := range rows {
		result[i] = make([]InlineKeyboardButton, len(row))
		for j, btn := range row {
			result[i][j] = InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.CallbackData,
			}
		}
	}
	
	return InlineKeyboard{InlineKeyboard: result}
}

// ModelKeyboard returns an inline keyboard with model selection buttons
func ModelKeyboard() InlineKeyboard {
	return BuildInlineKeyboardRows([][]InlineButton{
		{
			{Text: "🎭 Sonnet 4", CallbackData: "model:claude-sonnet-4-20250514"},
			{Text: "🎩 Opus 4", CallbackData: "model:claude-opus-4-20250514"},
		},
		{
			{Text: "🎭 Sonnet 3.5", CallbackData: "model:claude-3-5-sonnet-20241022"},
			{Text: "⚡ Haiku 3", CallbackData: "model:claude-3-haiku-20240307"},
		},
	})
}

// NewChatKeyboard returns a confirmation keyboard for /new command
func NewChatKeyboard() InlineKeyboard {
	return BuildInlineKeyboard([]InlineButton{
		{Text: "✅ Started new chat", CallbackData: "new:confirm"},
	})
}

// ParseCallbackData parses callback data in the format "action:value"
func ParseCallbackData(data string) (action, value string) {
	parts := strings.SplitN(data, ":", 2)
	action = parts[0]
	if len(parts) > 1 {
		value = parts[1]
	}
	return
}

// FormatModelName returns a display-friendly model name
func FormatModelName(model string) string {
	switch model {
	case "claude-sonnet-4-20250514":
		return "Sonnet 4"
	case "claude-opus-4-20250514":
		return "Opus 4"
	case "claude-3-5-sonnet-20241022":
		return "Sonnet 3.5"
	case "claude-3-opus-20240229":
		return "Opus 3"
	case "claude-3-sonnet-20240229":
		return "Sonnet 3"
	case "claude-3-haiku-20240307":
		return "Haiku 3"
	default:
		return model
	}
}
