package channel

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBotCommandsList(t *testing.T) {
	commands := BotCommands()
	
	expected := []BotCommand{
		{Command: "new", Description: "Start a new conversation"},
		{Command: "history", Description: "Show recent messages"},
		{Command: "model", Description: "Show or switch AI model"},
		{Command: "usage", Description: "Show token usage stats"},
		{Command: "reminders", Description: "List active reminders"},
		{Command: "help", Description: "Show all commands"},
	}
	
	if len(commands) != len(expected) {
		t.Errorf("Expected %d commands, got %d", len(expected), len(commands))
	}
	
	for i, cmd := range expected {
		if i >= len(commands) {
			break
		}
		if commands[i].Command != cmd.Command {
			t.Errorf("Command %d: expected %q, got %q", i, cmd.Command, commands[i].Command)
		}
		if commands[i].Description != cmd.Description {
			t.Errorf("Description for /%s: expected %q, got %q", cmd.Command, cmd.Description, commands[i].Description)
		}
	}
}

func TestSetMyCommandsPayload(t *testing.T) {
	commands := BotCommands()
	
	// Verify JSON marshaling works correctly
	payload := map[string]any{
		"commands": commands,
	}
	
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal commands: %v", err)
	}
	
	// Verify the JSON structure
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	
	if _, ok := parsed["commands"]; !ok {
		t.Error("Expected 'commands' key in payload")
	}
}

func TestInlineKeyboardBuilder(t *testing.T) {
	tests := []struct {
		name     string
		buttons  []InlineButton
		expected InlineKeyboard
	}{
		{
			name: "single row",
			buttons: []InlineButton{
				{Text: "Button 1", CallbackData: "btn1"},
				{Text: "Button 2", CallbackData: "btn2"},
			},
			expected: InlineKeyboard{
				InlineKeyboard: [][]InlineKeyboardButton{
					{
						{Text: "Button 1", CallbackData: "btn1"},
						{Text: "Button 2", CallbackData: "btn2"},
					},
				},
			},
		},
		{
			name:    "empty",
			buttons: []InlineButton{},
			expected: InlineKeyboard{
				InlineKeyboard: [][]InlineKeyboardButton{},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildInlineKeyboard(tt.buttons)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestInlineKeyboardRows(t *testing.T) {
	rows := [][]InlineButton{
		{{Text: "Row 1 Btn 1", CallbackData: "r1b1"}, {Text: "Row 1 Btn 2", CallbackData: "r1b2"}},
		{{Text: "Row 2 Btn 1", CallbackData: "r2b1"}},
	}
	
	result := BuildInlineKeyboardRows(rows)
	
	if len(result.InlineKeyboard) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(result.InlineKeyboard))
	}
	
	if len(result.InlineKeyboard[0]) != 2 {
		t.Errorf("Expected 2 buttons in row 1, got %d", len(result.InlineKeyboard[0]))
	}
	
	if len(result.InlineKeyboard[1]) != 1 {
		t.Errorf("Expected 1 button in row 2, got %d", len(result.InlineKeyboard[1]))
	}
}

func TestModelKeyboard(t *testing.T) {
	keyboard := ModelKeyboard()
	
	if len(keyboard.InlineKeyboard) != 2 {
		t.Errorf("Expected 2 rows of model buttons, got %d", len(keyboard.InlineKeyboard))
	}
	
	// Check we have sonnet and opus in first row
	row1 := keyboard.InlineKeyboard[0]
	foundSonnet := false
	foundOpus := false
	for _, btn := range row1 {
		if btn.CallbackData == "model:claude-sonnet-4-20250514" {
			foundSonnet = true
		}
		if btn.CallbackData == "model:claude-opus-4-20250514" {
			foundOpus = true
		}
	}
	
	if !foundSonnet {
		t.Error("Expected sonnet model button in keyboard")
	}
	if !foundOpus {
		t.Error("Expected opus model button in keyboard")
	}
}

func TestNewChatKeyboard(t *testing.T) {
	keyboard := NewChatKeyboard()
	
	if len(keyboard.InlineKeyboard) != 1 {
		t.Errorf("Expected 1 row, got %d", len(keyboard.InlineKeyboard))
	}
	
	if len(keyboard.InlineKeyboard[0]) != 1 {
		t.Errorf("Expected 1 button, got %d", len(keyboard.InlineKeyboard[0]))
	}
	
	btn := keyboard.InlineKeyboard[0][0]
	if btn.Text != "✅ Started new chat" {
		t.Errorf("Unexpected button text: %s", btn.Text)
	}
}

func TestParseCallbackData(t *testing.T) {
	tests := []struct {
		input    string
		action   string
		value    string
	}{
		{"model:claude-sonnet-4-20250514", "model", "claude-sonnet-4-20250514"},
		{"new:confirm", "new", "confirm"},
		{"simple", "simple", ""},
		{"a:b:c", "a", "b:c"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, value := ParseCallbackData(tt.input)
			if action != tt.action {
				t.Errorf("action: expected %q, got %q", tt.action, action)
			}
			if value != tt.value {
				t.Errorf("value: expected %q, got %q", tt.value, value)
			}
		})
	}
}

func TestCallbackQueryStruct(t *testing.T) {
	// Test that TelegramUpdate with callback_query deserializes correctly
	jsonData := `{
		"update_id": 123,
		"callback_query": {
			"id": "query123",
			"from": {"id": 456, "first_name": "Test"},
			"message": {"message_id": 789, "chat": {"id": 111, "type": "private"}},
			"data": "model:claude-sonnet-4-20250514"
		}
	}`
	
	var update TelegramUpdate
	if err := json.Unmarshal([]byte(jsonData), &update); err != nil {
		t.Fatalf("Failed to unmarshal callback_query update: %v", err)
	}
	
	if update.CallbackQuery == nil {
		t.Fatal("CallbackQuery should not be nil")
	}
	
	if update.CallbackQuery.ID != "query123" {
		t.Errorf("Expected ID 'query123', got %q", update.CallbackQuery.ID)
	}
	
	if update.CallbackQuery.Data != "model:claude-sonnet-4-20250514" {
		t.Errorf("Expected data 'model:claude-sonnet-4-20250514', got %q", update.CallbackQuery.Data)
	}
	
	if update.CallbackQuery.Message == nil {
		t.Fatal("Message in callback should not be nil")
	}
}

func TestFormatModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4-20250514", "Sonnet 4"},
		{"claude-opus-4-20250514", "Opus 4"},
		{"claude-3-5-sonnet-20241022", "Sonnet 3.5"},
		{"claude-3-haiku-20240307", "Haiku 3"},
		{"unknown-model", "unknown-model"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := FormatModelName(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
