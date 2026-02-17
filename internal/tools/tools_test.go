package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()

	tool := &Tool{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters: []Parameter{
			{Name: "arg1", Type: "string", Required: true},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			return "result", nil
		},
	}

	reg.Register(tool)

	got := reg.Get("test_tool")
	if got == nil {
		t.Fatal("Get returned nil for registered tool")
	}
	if got.Name != "test_tool" {
		t.Errorf("Got tool name %s, want test_tool", got.Name)
	}

	// Non-existent tool
	got = reg.Get("nonexistent")
	if got != nil {
		t.Error("Expected nil for non-existent tool")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()

	reg.Register(&Tool{Name: "tool1", Description: "First tool"})
	reg.Register(&Tool{Name: "tool2", Description: "Second tool"})

	tools := reg.List()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
}

func TestToolToAnthropicSchema(t *testing.T) {
	tool := &Tool{
		Name:        "web_search",
		Description: "Search the web for information",
		Parameters: []Parameter{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "limit", Type: "integer", Description: "Max results", Required: false},
		},
	}

	schema := tool.ToAnthropicSchema()

	if schema["name"] != "web_search" {
		t.Errorf("name = %v, want web_search", schema["name"])
	}
	if schema["description"] != "Search the web for information" {
		t.Errorf("description = %v, want 'Search the web for information'", schema["description"])
	}

	inputSchema := schema["input_schema"].(map[string]any)
	if inputSchema["type"] != "object" {
		t.Errorf("input_schema.type = %v, want object", inputSchema["type"])
	}

	props := inputSchema["properties"].(map[string]any)
	queryProp := props["query"].(map[string]any)
	if queryProp["type"] != "string" {
		t.Errorf("query type = %v, want string", queryProp["type"])
	}

	required := inputSchema["required"].([]string)
	if len(required) != 1 || required[0] != "query" {
		t.Errorf("required = %v, want [query]", required)
	}
}

func TestToolToOpenAISchema(t *testing.T) {
	tool := &Tool{
		Name:        "exec",
		Description: "Execute a shell command",
		Parameters: []Parameter{
			{Name: "command", Type: "string", Description: "Command to run", Required: true},
		},
	}

	schema := tool.ToOpenAISchema()

	if schema["type"] != "function" {
		t.Errorf("type = %v, want function", schema["type"])
	}

	fn := schema["function"].(map[string]any)
	if fn["name"] != "exec" {
		t.Errorf("function.name = %v, want exec", fn["name"])
	}
}

func TestExecTool(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	execTool := reg.Get("exec")
	if execTool == nil {
		t.Fatal("exec tool not found")
	}

	// Test simple echo command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := execTool.Handler(ctx, map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}

	if !strings.Contains(result, "hello") {
		t.Errorf("Expected result to contain 'hello', got: %s", result)
	}
}

func TestExecToolTimeout(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	execTool := reg.Get("exec")

	// Test with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := execTool.Handler(ctx, map[string]any{
		"command": "sleep 10",
	})

	// Should timeout or be cancelled
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestWebSearchToolRegistered(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	searchTool := reg.Get("web_search")
	if searchTool == nil {
		t.Fatal("web_search tool not found")
	}

	if searchTool.Description == "" {
		t.Error("web_search should have a description")
	}
}
