package subagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/internal/tools"
)

// MockAgentRunner is a mock implementation of AgentRunner for testing
type MockAgentRunner struct {
	RunTaskFunc func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error)
	delay       time.Duration
}

func (m *MockAgentRunner) RunTask(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if m.RunTaskFunc != nil {
		return m.RunTaskFunc(ctx, task, systemPrompt, toolRegistry)
	}
	return "Mock result for: " + task, nil
}

func TestManagerSpawnAndComplete(t *testing.T) {
	var completedID, completedLabel, completedResult, completedParent string
	var completedErr error
	var wg sync.WaitGroup
	wg.Add(1)

	onComplete := func(agentID, label, result, parentKey string, err error) {
		completedID = agentID
		completedLabel = label
		completedResult = result
		completedParent = parentKey
		completedErr = err
		wg.Done()
	}

	manager := NewManager(onComplete)

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "Completed: " + task, nil
		},
	}

	// Spawn an agent
	agentID := manager.Spawn("test task", "test-agent", "", "telegram:123", runner, nil)

	if agentID == "" {
		t.Fatal("Expected non-empty agent ID")
	}

	// Wait for completion
	wg.Wait()

	// Verify completion callback was called correctly
	if completedID != agentID {
		t.Errorf("Expected completedID=%s, got %s", agentID, completedID)
	}
	if completedLabel != "test-agent" {
		t.Errorf("Expected label=test-agent, got %s", completedLabel)
	}
	if completedResult != "Completed: test task" {
		t.Errorf("Expected result='Completed: test task', got %s", completedResult)
	}
	if completedParent != "telegram:123" {
		t.Errorf("Expected parent=telegram:123, got %s", completedParent)
	}
	if completedErr != nil {
		t.Errorf("Expected no error, got %v", completedErr)
	}

	// Verify agent status
	agent, exists := manager.Get(agentID)
	if !exists {
		t.Fatal("Agent should exist")
	}

	agent.mu.RLock()
	status := agent.Status
	result := agent.Result
	agent.mu.RUnlock()

	if status != StatusDone {
		t.Errorf("Expected status=done, got %s", status)
	}
	if result != "Completed: test task" {
		t.Errorf("Expected result='Completed: test task', got %s", result)
	}
}

func TestManagerSpawnWithError(t *testing.T) {
	var completedErr error
	var wg sync.WaitGroup
	wg.Add(1)

	onComplete := func(agentID, label, result, parentKey string, err error) {
		completedErr = err
		wg.Done()
	}

	manager := NewManager(onComplete)

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "", errors.New("test error")
		},
	}

	agentID := manager.Spawn("failing task", "fail-agent", "", "telegram:123", runner, nil)

	// Wait for completion
	wg.Wait()

	// Verify error was reported
	if completedErr == nil {
		t.Error("Expected error in completion callback")
	}
	if completedErr.Error() != "test error" {
		t.Errorf("Expected error='test error', got %v", completedErr)
	}

	// Verify agent status
	agent, _ := manager.Get(agentID)
	agent.mu.RLock()
	status := agent.Status
	errMsg := agent.Error
	agent.mu.RUnlock()

	if status != StatusFailed {
		t.Errorf("Expected status=failed, got %s", status)
	}
	if errMsg != "test error" {
		t.Errorf("Expected error='test error', got %s", errMsg)
	}
}

func TestManagerCancel(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	onComplete := func(agentID, label, result, parentKey string, err error) {
		wg.Done()
	}

	manager := NewManager(onComplete)
	manager.SetMaxRuntime(5 * time.Second)

	runner := &MockAgentRunner{
		delay: 10 * time.Second, // Will be canceled before this
	}

	agentID := manager.Spawn("long task", "slow-agent", "", "telegram:123", runner, nil)

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Cancel the agent
	err := manager.Cancel(agentID)
	if err != nil {
		t.Errorf("Cancel failed: %v", err)
	}

	// Wait for completion callback
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for cancel to complete")
	}

	// Verify status
	agent, _ := manager.Get(agentID)
	agent.mu.RLock()
	status := agent.Status
	agent.mu.RUnlock()

	if status != StatusCanceled {
		t.Errorf("Expected status=canceled, got %s", status)
	}
}

func TestManagerCancelNotFound(t *testing.T) {
	manager := NewManager(nil)

	err := manager.Cancel("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent agent")
	}
}

func TestManagerList(t *testing.T) {
	completed := make(chan struct{}, 3)

	onComplete := func(agentID, label, result, parentKey string, err error) {
		completed <- struct{}{}
	}

	manager := NewManager(onComplete)

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "done", nil
		},
	}

	// Spawn multiple agents
	manager.Spawn("task1", "agent1", "", "telegram:1", runner, nil)
	manager.Spawn("task2", "agent2", "", "telegram:2", runner, nil)
	manager.Spawn("task3", "agent3", "", "telegram:3", runner, nil)

	// Wait for all to complete
	for i := 0; i < 3; i++ {
		<-completed
	}

	// List all agents
	agents := manager.List()
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}

	// List by status
	doneAgents := manager.List(StatusDone)
	if len(doneAgents) != 3 {
		t.Errorf("Expected 3 done agents, got %d", len(doneAgents))
	}

	runningAgents := manager.List(StatusRunning)
	if len(runningAgents) != 0 {
		t.Errorf("Expected 0 running agents, got %d", len(runningAgents))
	}
}

func TestManagerCleanup(t *testing.T) {
	completed := make(chan struct{})

	onComplete := func(agentID, label, result, parentKey string, err error) {
		completed <- struct{}{}
	}

	manager := NewManager(onComplete)

	runner := &MockAgentRunner{
		RunTaskFunc: func(ctx context.Context, task, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
			return "done", nil
		},
	}

	agentID := manager.Spawn("task", "agent", "", "telegram:1", runner, nil)

	// Wait for completion
	<-completed

	// Cleanup with 0 maxAge should remove completed agents
	removed := manager.Cleanup(0)
	if removed != 1 {
		t.Errorf("Expected 1 removed, got %d", removed)
	}

	// Agent should no longer exist
	_, exists := manager.Get(agentID)
	if exists {
		t.Error("Agent should have been cleaned up")
	}
}

func TestFilterTools(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(&tools.Tool{Name: "exec", Description: "Execute command"})
	registry.Register(&tools.Tool{Name: "spawn_agent", Description: "Spawn sub-agent"})
	registry.Register(&tools.Tool{Name: "web_search", Description: "Search web"})

	filtered := filterTools(registry)

	// spawn_agent should be filtered out
	if filtered.Get("spawn_agent") != nil {
		t.Error("spawn_agent should be filtered out")
	}

	// Other tools should remain
	if filtered.Get("exec") == nil {
		t.Error("exec should remain")
	}
	if filtered.Get("web_search") == nil {
		t.Error("web_search should remain")
	}

	// Count should be 2 (not 3)
	if len(filtered.List()) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(filtered.List()))
	}
}

func TestSubAgentGetStatus(t *testing.T) {
	agent := &SubAgent{
		ID:        "sa-123",
		Label:     "test-agent",
		Task:      "Do something important",
		Status:    StatusDone,
		Result:    "Task completed successfully",
		StartedAt: time.Now().Add(-5 * time.Minute),
		CompletedAt: time.Now(),
	}

	status := agent.GetStatus()

	// Verify key elements are present
	if !contains(status, "sa-123") {
		t.Error("Status should contain agent ID")
	}
	if !contains(status, "test-agent") {
		t.Error("Status should contain label")
	}
	if !contains(status, "Do something important") {
		t.Error("Status should contain task")
	}
	if !contains(status, "Done") {
		t.Error("Status should contain status")
	}
	if !contains(status, "Task completed successfully") {
		t.Error("Status should contain result")
	}
}

func TestManagerTimeout(t *testing.T) {
	var completedErr error
	var wg sync.WaitGroup
	wg.Add(1)

	onComplete := func(agentID, label, result, parentKey string, err error) {
		completedErr = err
		wg.Done()
	}

	manager := NewManager(onComplete)
	manager.SetMaxRuntime(100 * time.Millisecond) // Very short timeout

	runner := &MockAgentRunner{
		delay: 5 * time.Second, // Much longer than timeout
	}

	manager.Spawn("slow task", "slow-agent", "", "telegram:123", runner, nil)

	// Wait for completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for agent to timeout")
	}

	// Should have failed due to timeout
	if completedErr == nil {
		t.Error("Expected timeout error")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
