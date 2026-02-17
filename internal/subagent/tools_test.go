package subagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/internal/tools"
)

func TestSpawnAgentTool(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	manager := NewManager(func(agentID, label, result, parentKey string, err error) {
		wg.Done()
	})

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "Done: " + task, nil
		},
	}

	// Create tool context
	toolCtx := &ToolContext{
		Manager:          manager,
		ParentSessionKey: "telegram:12345",
		AgentRunner:      runner,
		ToolRegistry:     tools.NewRegistry(),
	}

	// Register tools
	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	// Get spawn_agent tool
	spawnTool := registry.Get("spawn_agent")
	if spawnTool == nil {
		t.Fatal("spawn_agent tool not registered")
	}

	// Test spawn with valid params
	result, err := spawnTool.Handler(context.Background(), map[string]any{
		"task":  "Test task",
		"label": "test-label",
	})

	if err != nil {
		t.Fatalf("spawn_agent failed: %v", err)
	}

	if result == "" {
		t.Fatal("Expected non-empty result")
	}

	if !containsString(result, "Sub-agent spawned") {
		t.Errorf("Expected success message, got: %s", result)
	}

	// Wait for agent to complete
	wg.Wait()
}

func TestSpawnAgentToolMissingParams(t *testing.T) {
	manager := NewManager(nil)
	runner := &MockAgentRunner{}

	toolCtx := &ToolContext{
		Manager:     manager,
		AgentRunner: runner,
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	spawnTool := registry.Get("spawn_agent")

	// Test with missing task
	_, err := spawnTool.Handler(context.Background(), map[string]any{
		"label": "test",
	})
	if err == nil {
		t.Error("Expected error for missing task")
	}

	// Test with missing label
	_, err = spawnTool.Handler(context.Background(), map[string]any{
		"task": "test",
	})
	if err == nil {
		t.Error("Expected error for missing label")
	}
}

func TestAgentStatusTool(t *testing.T) {
	completed := make(chan struct{})

	manager := NewManager(func(agentID, label, result, parentKey string, err error) {
		completed <- struct{}{}
	})

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "Done", nil
		},
	}

	toolCtx := &ToolContext{
		Manager:      manager,
		AgentRunner:  runner,
		ToolRegistry: tools.NewRegistry(),
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	// Spawn an agent first
	spawnTool := registry.Get("spawn_agent")
	spawnTool.Handler(context.Background(), map[string]any{
		"task":  "Test task",
		"label": "status-test",
	})

	<-completed

	// Test agent_status with no agent_id (list all)
	statusTool := registry.Get("agent_status")
	result, err := statusTool.Handler(context.Background(), map[string]any{})

	if err != nil {
		t.Fatalf("agent_status failed: %v", err)
	}

	if !containsString(result, "Sub-agents") {
		t.Errorf("Expected agents list, got: %s", result)
	}

	if !containsString(result, "status-test") {
		t.Errorf("Expected agent label in result, got: %s", result)
	}
}

func TestAgentStatusToolSpecificAgent(t *testing.T) {
	completed := make(chan string, 1)

	manager := NewManager(func(agentID, label, result, parentKey string, err error) {
		completed <- agentID
	})

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "Result here", nil
		},
	}

	toolCtx := &ToolContext{
		Manager:      manager,
		AgentRunner:  runner,
		ToolRegistry: tools.NewRegistry(),
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	// Get the spawn tool and spawn an agent
	spawnTool := registry.Get("spawn_agent")
	spawnResult, _ := spawnTool.Handler(context.Background(), map[string]any{
		"task":  "Test task for specific agent",
		"label": "specific-agent",
	})

	// Extract agent ID from spawn result
	agentID := <-completed

	// Get status for specific agent
	statusTool := registry.Get("agent_status")
	result, err := statusTool.Handler(context.Background(), map[string]any{
		"agent_id": agentID,
	})

	if err != nil {
		t.Fatalf("agent_status failed: %v", err)
	}

	if !containsString(result, agentID) {
		t.Errorf("Expected agent ID in result, got: %s (spawned: %s)", result, spawnResult)
	}

	if !containsString(result, "specific-agent") {
		t.Errorf("Expected label in result, got: %s", result)
	}
}

func TestAgentStatusToolNotFound(t *testing.T) {
	manager := NewManager(nil)

	toolCtx := &ToolContext{
		Manager: manager,
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	statusTool := registry.Get("agent_status")
	result, err := statusTool.Handler(context.Background(), map[string]any{
		"agent_id": "nonexistent",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !containsString(result, "not found") {
		t.Errorf("Expected 'not found' message, got: %s", result)
	}
}

func TestCancelAgentTool(t *testing.T) {
	cancelCalled := make(chan struct{})

	manager := NewManager(func(agentID, label, result, parentKey string, err error) {
		cancelCalled <- struct{}{}
	})

	runner := &MockAgentRunner{
		delay: 5 * time.Second, // Long running task
	}

	toolCtx := &ToolContext{
		Manager:      manager,
		AgentRunner:  runner,
		ToolRegistry: tools.NewRegistry(),
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	// Spawn an agent
	spawnTool := registry.Get("spawn_agent")
	spawnTool.Handler(context.Background(), map[string]any{
		"task":  "Long running task",
		"label": "cancel-test",
	})

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Get the agent ID
	agents := manager.List()
	if len(agents) == 0 {
		t.Fatal("No agents found")
	}
	agentID := agents[0].ID

	// Cancel it
	cancelTool := registry.Get("cancel_agent")
	result, err := cancelTool.Handler(context.Background(), map[string]any{
		"agent_id": agentID,
	})

	if err != nil {
		t.Fatalf("cancel_agent failed: %v", err)
	}

	if !containsString(result, "canceled") {
		t.Errorf("Expected 'canceled' in result, got: %s", result)
	}

	// Wait for completion callback
	select {
	case <-cancelCalled:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Cancel callback not called")
	}

	// Verify status
	agent, _ := manager.Get(agentID)
	status, _, _, _, _ := agent.GetInfo()
	if status != StatusCanceled {
		t.Errorf("Expected status=canceled, got %s", status)
	}
}

func TestCancelAgentToolMissingID(t *testing.T) {
	manager := NewManager(nil)

	toolCtx := &ToolContext{
		Manager: manager,
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	cancelTool := registry.Get("cancel_agent")
	_, err := cancelTool.Handler(context.Background(), map[string]any{})

	if err == nil {
		t.Error("Expected error for missing agent_id")
	}
}

func TestCancelAgentToolNotFound(t *testing.T) {
	manager := NewManager(nil)

	toolCtx := &ToolContext{
		Manager: manager,
	}

	registry := tools.NewRegistry()
	RegisterTools(registry, toolCtx)

	cancelTool := registry.Get("cancel_agent")
	result, err := cancelTool.Handler(context.Background(), map[string]any{
		"agent_id": "nonexistent",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !containsString(result, "not found") {
		t.Errorf("Expected 'not found' message, got: %s", result)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
