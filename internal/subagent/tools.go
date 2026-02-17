package subagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/tools"
)

// ToolContext provides context for sub-agent tools
type ToolContext struct {
	Manager          *Manager
	ParentSessionKey string
	AgentRunner      AgentRunner
	ToolRegistry     *tools.Registry
}

// RegisterTools registers sub-agent tools with the provided registry
func RegisterTools(registry *tools.Registry, ctx *ToolContext) {
	// spawn_agent tool
	registry.Register(&tools.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a background sub-agent to work on a task autonomously. The agent will run independently and inject its result back into this conversation when done.",
		Parameters: []tools.Parameter{
			{Name: "task", Type: "string", Description: "The task for the sub-agent to complete", Required: true},
			{Name: "label", Type: "string", Description: "Short label to identify this sub-agent", Required: true},
			{Name: "system_prompt", Type: "string", Description: "Optional system prompt override for the sub-agent", Required: false},
		},
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			return handleSpawnAgent(ctx, params)
		},
	})

	// agent_status tool
	registry.Register(&tools.Tool{
		Name:        "agent_status",
		Description: "Check status of spawned sub-agents",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to check, or omit for all agents", Required: false},
		},
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			return handleAgentStatus(ctx, params)
		},
	})

	// cancel_agent tool
	registry.Register(&tools.Tool{
		Name:        "cancel_agent",
		Description: "Cancel a running sub-agent",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to cancel", Required: true},
		},
		Handler: func(_ context.Context, params map[string]any) (string, error) {
			return handleCancelAgent(ctx, params)
		},
	})
}

// handleSpawnAgent handles the spawn_agent tool
func handleSpawnAgent(ctx *ToolContext, params map[string]any) (string, error) {
	if ctx.Manager == nil {
		return "", fmt.Errorf("sub-agent manager not available")
	}

	task, ok := params["task"].(string)
	if !ok || task == "" {
		return "", fmt.Errorf("task is required")
	}

	label, ok := params["label"].(string)
	if !ok || label == "" {
		return "", fmt.Errorf("label is required")
	}

	systemPrompt, _ := params["system_prompt"].(string)

	if ctx.AgentRunner == nil {
		return "", fmt.Errorf("agent runner not configured")
	}

	// Spawn the agent
	agentID := ctx.Manager.Spawn(
		task,
		label,
		systemPrompt,
		ctx.ParentSessionKey,
		ctx.AgentRunner,
		ctx.ToolRegistry,
	)

	return fmt.Sprintf("✅ Sub-agent spawned!\n\nID: %s\nLabel: %s\nTask: %s\n\nThe agent is now running in the background. You'll be notified when it completes.",
		agentID, label, truncate(task, 100)), nil
}

// handleAgentStatus handles the agent_status tool
func handleAgentStatus(ctx *ToolContext, params map[string]any) (string, error) {
	if ctx.Manager == nil {
		return "", fmt.Errorf("sub-agent manager not available")
	}

	// If agent_id provided, show specific agent
	if agentID, ok := params["agent_id"].(string); ok && agentID != "" {
		agent, exists := ctx.Manager.Get(agentID)
		if !exists {
			return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
		}
		return agent.GetStatus(), nil
	}

	// Show all agents
	agents := ctx.Manager.List()
	if len(agents) == 0 {
		return "📭 No sub-agents have been spawned.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 *Sub-agents* (%d)\n\n", len(agents)))

	for _, agent := range agents {
		agent.mu.RLock()
		status := formatStatus(agent.Status)
		label := agent.Label
		id := agent.ID
		task := truncate(agent.Task, 50)
		agent.mu.RUnlock()

		sb.WriteString(fmt.Sprintf("• `%s` (%s) — %s\n  Task: %s\n\n", id, label, status, task))
	}

	return sb.String(), nil
}

// handleCancelAgent handles the cancel_agent tool
func handleCancelAgent(ctx *ToolContext, params map[string]any) (string, error) {
	if ctx.Manager == nil {
		return "", fmt.Errorf("sub-agent manager not available")
	}

	agentID, ok := params["agent_id"].(string)
	if !ok || agentID == "" {
		return "", fmt.Errorf("agent_id is required")
	}

	// Get agent info before canceling
	agent, exists := ctx.Manager.Get(agentID)
	if !exists {
		return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
	}

	agent.mu.RLock()
	label := agent.Label
	agent.mu.RUnlock()

	if err := ctx.Manager.Cancel(agentID); err != nil {
		return fmt.Sprintf("❌ Failed to cancel agent: %v", err), nil
	}

	return fmt.Sprintf("🚫 Sub-agent canceled!\n\nID: %s\nLabel: %s", agentID, label), nil
}
