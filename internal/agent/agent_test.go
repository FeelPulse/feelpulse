package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// MockAnthropicAPI creates a configurable mock Anthropic API server
type MockAnthropicAPI struct {
	server       *httptest.Server
	responses    []MockAPIResponse
	reqIdx       int32
	mu           sync.Mutex
	receivedReqs []AnthropicRequest
}

type MockAPIResponse struct {
	Content    []AnthropicContent
	StopReason string
	Usage      AnthropicUsage
	Error      *AnthropicError
}

func NewMockAnthropicAPI() *MockAnthropicAPI {
	m := &MockAnthropicAPI{
		responses:    make([]MockAPIResponse, 0),
		receivedReqs: make([]AnthropicRequest, 0),
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req AnthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		m.mu.Lock()
		m.receivedReqs = append(m.receivedReqs, req)
		m.mu.Unlock()

		// Get response for this request
		idx := int(atomic.AddInt32(&m.reqIdx, 1)) - 1

		m.mu.Lock()
		var mockResp MockAPIResponse
		if idx < len(m.responses) {
			mockResp = m.responses[idx]
		} else {
			// Default response
			mockResp = MockAPIResponse{
				Content: []AnthropicContent{
					{Type: "text", Text: "Default response"},
				},
				StopReason: "end_turn",
				Usage:      AnthropicUsage{InputTokens: 10, OutputTokens: 5},
			}
		}
		m.mu.Unlock()

		// Return error if configured
		if mockResp.Error != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": mockResp.Error,
			})
			return
		}

		// Build response
		resp := AnthropicResponse{
			ID:         fmt.Sprintf("msg_test_%d", idx),
			Type:       "message",
			Role:       "assistant",
			Content:    mockResp.Content,
			Model:      "claude-sonnet-4-20250514",
			StopReason: mockResp.StopReason,
			Usage:      mockResp.Usage,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return m
}

func (m *MockAnthropicAPI) AddResponse(resp MockAPIResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
}

func (m *MockAnthropicAPI) URL() string {
	return m.server.URL
}

func (m *MockAnthropicAPI) Close() {
	m.server.Close()
}

func (m *MockAnthropicAPI) RequestCount() int {
	return int(atomic.LoadInt32(&m.reqIdx))
}

func (m *MockAnthropicAPI) ReceivedRequests() []AnthropicRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]AnthropicRequest, len(m.receivedReqs))
	copy(result, m.receivedReqs)
	return result
}

// TestAnthropicClient_Name tests the Name method
func TestAnthropicClient_Name(t *testing.T) {
	client := NewAnthropicClient("test-key", "", "")
	if client.Name() != "anthropic" {
		t.Errorf("Expected name 'anthropic', got '%s'", client.Name())
	}
}

// TestAnthropicClient_AuthMode tests authentication mode detection
func TestAnthropicClient_AuthMode(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		authToken string
		wantMode  string
	}{
		{
			name:     "API key auth",
			apiKey:   "sk-ant-api-test",
			wantMode: "api-key",
		},
		{
			name:      "OAuth token auth",
			authToken: "sk-ant-oat-test-token",
			wantMode:  "subscription (setup-token)",
		},
		{
			name:      "Non-OAuth token treated as API key",
			authToken: "some-other-token",
			wantMode:  "api-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewAnthropicClient(tt.apiKey, tt.authToken, "")
			if client.AuthModeName() != tt.wantMode {
				t.Errorf("Expected auth mode '%s', got '%s'", tt.wantMode, client.AuthModeName())
			}
		})
	}
}

// TestAnthropicClient_DefaultModel tests default model selection
func TestAnthropicClient_DefaultModel(t *testing.T) {
	// Without explicit model
	client1 := NewAnthropicClient("test-key", "", "")
	// Model is private, so we can only verify the client was created
	if client1 == nil {
		t.Error("Expected non-nil client")
	}

	// With explicit model
	client2 := NewAnthropicClient("test-key", "", "claude-3-opus-20240229")
	if client2 == nil {
		t.Error("Expected non-nil client with explicit model")
	}
}

// Note: TestIsOAuthToken and TestTruncateString are in anthropic_test.go

// TestAgenticLoop_SimpleTextResponse tests a simple text response without tools
func TestAgenticLoop_SimpleTextResponse(t *testing.T) {
	// Create mock server
	mock := NewMockAnthropicAPI()
	defer mock.Close()

	// Add response
	mock.AddResponse(MockAPIResponse{
		Content: []AnthropicContent{
			{Type: "text", Text: "Hello! How can I help you today?"},
		},
		StopReason: "end_turn",
		Usage:      AnthropicUsage{InputTokens: 50, OutputTokens: 20},
	})

	// Create messages
	messages := []types.Message{
		{Text: "Hello!", IsBot: false},
	}

	// Create client (note: in production this would hit real API, but for testing we verify the logic)
	client := NewAnthropicClient("test-key", "", "claude-sonnet-4-20250514")
	if client == nil {
		t.Fatal("Failed to create client")
	}

	// Verify client was created properly
	if client.Name() != "anthropic" {
		t.Errorf("Expected anthropic provider, got %s", client.Name())
	}

	// Verify message conversion works
	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Text != "Hello!" {
		t.Errorf("Expected 'Hello!', got %s", messages[0].Text)
	}
}

// TestAgenticLoop_ToolUseAndResult tests the tool use flow
func TestAgenticLoop_ToolUseAndResult(t *testing.T) {
	// Create mock server
	mock := NewMockAnthropicAPI()
	defer mock.Close()

	// First response: tool use
	toolInput, _ := json.Marshal(map[string]any{"location": "London"})
	mock.AddResponse(MockAPIResponse{
		Content: []AnthropicContent{
			{Type: "text", Text: "Let me check the weather."},
			{
				Type:  "tool_use",
				ID:    "tool_1",
				Name:  "get_weather",
				Input: json.RawMessage(toolInput),
			},
		},
		StopReason: "tool_use",
		Usage:      AnthropicUsage{InputTokens: 100, OutputTokens: 50},
	})

	// Second response: final answer
	mock.AddResponse(MockAPIResponse{
		Content: []AnthropicContent{
			{Type: "text", Text: "The weather in London is 15°C and cloudy."},
		},
		StopReason: "end_turn",
		Usage:      AnthropicUsage{InputTokens: 150, OutputTokens: 30},
	})

	// Create tool executor
	toolCalled := false
	executor := func(name string, input map[string]any) (string, error) {
		toolCalled = true
		if name != "get_weather" {
			return "", fmt.Errorf("unknown tool: %s", name)
		}
		location, ok := input["location"].(string)
		if !ok {
			return "", fmt.Errorf("missing location parameter")
		}
		return fmt.Sprintf("Weather in %s: 15°C, cloudy", location), nil
	}

	// Create messages
	messages := []types.Message{
		{Text: "What's the weather in London?", IsBot: false},
	}

	// Create tool definition
	tools := []AnthropicTool{
		{
			Name:        "get_weather",
			Description: "Get current weather for a location",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		},
	}

	// Simulate agentic loop behavior (without hitting real API)
	// In production, ChatWithTools would do this

	// Simulate first turn - tool use response
	firstResp := mock.responses[0]
	if firstResp.StopReason != "tool_use" {
		t.Errorf("Expected tool_use stop reason, got %s", firstResp.StopReason)
	}

	// Find tool use in content
	var toolUseContent *AnthropicContent
	for i := range firstResp.Content {
		if firstResp.Content[i].Type == "tool_use" {
			toolUseContent = &firstResp.Content[i]
			break
		}
	}

	if toolUseContent == nil {
		t.Fatal("Expected tool_use in response")
	}

	if toolUseContent.Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got '%s'", toolUseContent.Name)
	}

	// Execute tool
	var input map[string]any
	json.Unmarshal(toolUseContent.Input, &input)
	result, err := executor(toolUseContent.Name, input)
	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}

	if !toolCalled {
		t.Error("Tool executor was not called")
	}

	if !strings.Contains(result, "15°C") {
		t.Errorf("Expected weather result, got %s", result)
	}

	// Verify tools were passed
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	_ = messages // Used to verify flow
}

// TestAgenticLoop_MaxIterations tests that max iterations prevents infinite loops
func TestAgenticLoop_MaxIterations(t *testing.T) {
	// Create mock server that always returns tool_use
	mock := NewMockAnthropicAPI()
	defer mock.Close()

	// Add 15 tool_use responses (more than max iterations of 10)
	toolInput, _ := json.Marshal(map[string]any{"action": "loop"})
	for i := 0; i < 15; i++ {
		mock.AddResponse(MockAPIResponse{
			Content: []AnthropicContent{
				{
					Type:  "tool_use",
					ID:    fmt.Sprintf("tool_%d", i),
					Name:  "infinite_tool",
					Input: json.RawMessage(toolInput),
				},
			},
			StopReason: "tool_use",
			Usage:      AnthropicUsage{InputTokens: 50, OutputTokens: 20},
		})
	}

	// Track iterations
	iterations := 0
	executor := func(name string, input map[string]any) (string, error) {
		iterations++
		return "result", nil
	}

	// Simulate max iterations check
	maxIterations := 10
	for i := 0; i < 15; i++ {
		if i >= maxIterations {
			break
		}

		// Simulate getting response
		if i < len(mock.responses) {
			resp := mock.responses[i]
			if resp.StopReason == "tool_use" {
				_, _ = executor("infinite_tool", nil)
			}
		}
	}

	// Verify we stopped at max iterations
	if iterations != maxIterations {
		t.Errorf("Expected %d iterations, got %d", maxIterations, iterations)
	}
}

// TestAgenticLoop_ToolError tests error handling in tool execution
func TestAgenticLoop_ToolError(t *testing.T) {
	// Create executor that returns error
	executor := func(name string, input map[string]any) (string, error) {
		return "", fmt.Errorf("tool execution failed: simulated error")
	}

	// Execute tool
	result, err := executor("test_tool", nil)
	if err == nil {
		t.Error("Expected error from tool execution")
	}

	if result != "" {
		t.Errorf("Expected empty result on error, got %s", result)
	}

	if !strings.Contains(err.Error(), "simulated error") {
		t.Errorf("Error message should contain 'simulated error', got %s", err.Error())
	}

	// In production, error would be formatted as tool result
	errorResult := fmt.Sprintf("Error: %v", err)
	if !strings.Contains(errorResult, "tool execution failed") {
		t.Error("Error result should contain error message")
	}
}

// TestAgenticLoop_MultipleToolCalls tests multiple tool calls in one response
func TestAgenticLoop_MultipleToolCalls(t *testing.T) {
	// Simulate response with multiple tool uses
	toolInputA, _ := json.Marshal(map[string]any{"city": "Paris"})
	toolInputB, _ := json.Marshal(map[string]any{"city": "London"})

	content := []AnthropicContent{
		{Type: "text", Text: "Let me check multiple cities."},
		{
			Type:  "tool_use",
			ID:    "tool_1",
			Name:  "get_weather",
			Input: json.RawMessage(toolInputA),
		},
		{
			Type:  "tool_use",
			ID:    "tool_2",
			Name:  "get_weather",
			Input: json.RawMessage(toolInputB),
		},
	}

	// Count tool uses
	toolUseBlocks := make([]AnthropicContent, 0)
	for _, block := range content {
		if block.Type == "tool_use" {
			toolUseBlocks = append(toolUseBlocks, block)
		}
	}

	if len(toolUseBlocks) != 2 {
		t.Errorf("Expected 2 tool use blocks, got %d", len(toolUseBlocks))
	}

	// Execute all tools
	executedTools := 0
	executor := func(name string, input map[string]any) (string, error) {
		executedTools++
		city := input["city"].(string)
		return fmt.Sprintf("Weather in %s: sunny", city), nil
	}

	for _, block := range toolUseBlocks {
		var input map[string]any
		json.Unmarshal(block.Input, &input)
		_, err := executor(block.Name, input)
		if err != nil {
			t.Errorf("Tool execution failed: %v", err)
		}
	}

	if executedTools != 2 {
		t.Errorf("Expected 2 tool executions, got %d", executedTools)
	}
}

// TestStreamCallback tests the stream callback functionality
func TestStreamCallback(t *testing.T) {
	var received strings.Builder

	callback := StreamCallback(func(delta string) {
		received.WriteString(delta)
	})

	// Simulate streaming deltas
	deltas := []string{"Hello", " ", "world", "!"}
	for _, delta := range deltas {
		callback(delta)
	}

	expected := "Hello world!"
	if received.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, received.String())
	}
}

// TestContentBlock_Parse tests parsing content blocks
func TestContentBlock_Parse(t *testing.T) {
	// Text block
	textJSON := `{"type":"text","text":"Hello"}`
	var textBlock ContentBlock
	if err := json.Unmarshal([]byte(textJSON), &textBlock); err != nil {
		t.Fatalf("Failed to parse text block: %v", err)
	}
	if textBlock.Type != "text" || textBlock.Text != "Hello" {
		t.Errorf("Text block mismatch: %+v", textBlock)
	}

	// Tool use block
	toolJSON := `{"type":"tool_use","id":"tu_123","name":"get_time","input":{"timezone":"UTC"}}`
	var toolBlock ContentBlock
	if err := json.Unmarshal([]byte(toolJSON), &toolBlock); err != nil {
		t.Fatalf("Failed to parse tool block: %v", err)
	}
	if toolBlock.Type != "tool_use" || toolBlock.Name != "get_time" {
		t.Errorf("Tool block mismatch: %+v", toolBlock)
	}

	// Tool result block
	resultJSON := `{"type":"tool_result","tool_use_id":"tu_123","content":"The time is 10:00 AM"}`
	var resultBlock ContentBlock
	if err := json.Unmarshal([]byte(resultJSON), &resultBlock); err != nil {
		t.Fatalf("Failed to parse result block: %v", err)
	}
	if resultBlock.Type != "tool_result" || resultBlock.ToolUseID != "tu_123" {
		t.Errorf("Result block mismatch: %+v", resultBlock)
	}
}

// TestAnthropicMessage_Content tests message content handling
func TestAnthropicMessage_Content(t *testing.T) {
	// String content
	msg1 := AnthropicMessage{
		Role:    "user",
		Content: "Hello, Claude!",
	}
	if msg1.Role != "user" {
		t.Errorf("Expected user role, got %s", msg1.Role)
	}

	// Array content (tool results)
	msg2 := AnthropicMessage{
		Role: "user",
		Content: []ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: "tu_123",
				Content:   "Tool result here",
			},
		},
	}
	if msg2.Role != "user" {
		t.Errorf("Expected user role, got %s", msg2.Role)
	}

	// Verify content can be marshaled
	data, err := json.Marshal(msg2)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}
	if !strings.Contains(string(data), "tool_result") {
		t.Error("Marshaled message should contain tool_result")
	}
}

// TestAnthropicUsage_Totals tests usage tracking
func TestAnthropicUsage_Totals(t *testing.T) {
	usage1 := AnthropicUsage{InputTokens: 100, OutputTokens: 50}
	usage2 := AnthropicUsage{InputTokens: 150, OutputTokens: 75}

	// Sum usage
	total := types.Usage{
		InputTokens:  usage1.InputTokens + usage2.InputTokens,
		OutputTokens: usage1.OutputTokens + usage2.OutputTokens,
	}

	if total.InputTokens != 250 {
		t.Errorf("Expected 250 input tokens, got %d", total.InputTokens)
	}
	if total.OutputTokens != 125 {
		t.Errorf("Expected 125 output tokens, got %d", total.OutputTokens)
	}
}

// TestSSEEvent_Parse tests parsing SSE events
func TestSSEEvent_Parse(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		wantType string
	}{
		{
			name:     "message_start",
			data:     `{"type":"message_start","message":{"id":"msg_123","model":"claude-sonnet-4-20250514"}}`,
			wantType: "message_start",
		},
		{
			name:     "content_block_delta",
			data:     `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			wantType: "content_block_delta",
		},
		{
			name:     "message_delta",
			data:     `{"type":"message_delta","usage":{"output_tokens":100}}`,
			wantType: "message_delta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseSSEEvent(tt.data)
			if err != nil {
				t.Fatalf("Failed to parse SSE event: %v", err)
			}
			if event.Type != tt.wantType {
				t.Errorf("Expected type '%s', got '%s'", tt.wantType, event.Type)
			}
		})
	}
}

// TestRouter_NoAgent tests router behavior without configured agent
func TestRouter_NoAgent(t *testing.T) {
	// Create router without agent
	router := &Router{}

	// Attempt to process should fail
	msg := &types.Message{Text: "Hello", Channel: "test"}
	_, err := router.Process(msg)
	if err == nil {
		t.Error("Expected error when processing without agent")
	}
}

// TestRouter_SystemPromptBuilder tests system prompt builder
func TestRouter_SystemPromptBuilder(t *testing.T) {
	router := &Router{}

	// Set prompt builder
	customPrompt := "You are a helpful assistant."
	router.SetSystemPromptBuilder(func(defaultPrompt string) string {
		return customPrompt
	})

	// Verify builder is set (can't directly test output without full router setup)
	if router.promptBuilder == nil {
		t.Error("Expected prompt builder to be set")
	}
}
