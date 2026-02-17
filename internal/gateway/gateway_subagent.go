package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/store"
	"github.com/FeelPulse/feelpulse/internal/subagent"
	"github.com/FeelPulse/feelpulse/internal/tools"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

func (gw *Gateway) initializeSubAgents() {
	if gw.subagentManager == nil {
		return
	}

	// Create completion callback that injects results and sends notifications
	onComplete := func(agentID, label, result, parentSessionKey string, duration time.Duration, err error) {
		gw.handleSubAgentComplete(agentID, label, result, parentSessionKey, duration, err)
	}

	// Create a new manager with the callback (replace the placeholder one)
	gw.subagentManager = subagent.NewManager(onComplete)

	// Wire up persistence via adapter
	if gw.db != nil {
		adapter := &subAgentPersisterAdapter{db: gw.db}
		if err := gw.subagentManager.SetPersister(adapter); err != nil {
			gw.log.Warn("Failed to set up sub-agent persistence: %v", err)
		} else {
			gw.log.Info("🤖 Sub-agent persistence enabled (SQLite)")
		}
	}

	// Register sub-agent tools if we have a router
	gw.mu.RLock()
	router := gw.router
	gw.mu.RUnlock()

	if router != nil && gw.toolRegistry != nil {
		gw.registerSubAgentTools()
		gw.log.Info("🤖 Sub-agent tools registered (spawn_agent, agent_status, cancel_agent)")
	}
}

// registerSubAgentTools registers spawn_agent, agent_status, cancel_agent tools
func (gw *Gateway) registerSubAgentTools() {
	if gw.subagentManager == nil {
		return
	}

	// Create the chat function that sub-agents will use
	chatFunc := gw.createSubAgentChatFunc()

	// Create runner
	runner := subagent.NewSimpleRunner(chatFunc, subagent.DefaultMaxIterations)

	// Register tools with a factory that creates context per session
	// We need to register tools that will get the parent session key at call time
	gw.toolRegistry.Register(&tools.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a background sub-agent to work on a task autonomously. The agent will run independently and inject its result back into this conversation when done.",
		Parameters: []tools.Parameter{
			{Name: "task", Type: "string", Description: "The task for the sub-agent to complete", Required: true},
			{Name: "label", Type: "string", Description: "Short label to identify this sub-agent", Required: true},
			{Name: "system_prompt", Type: "string", Description: "Optional system prompt override for the sub-agent", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			task, _ := params["task"].(string)
			label, _ := params["label"].(string)
			systemPrompt, _ := params["system_prompt"].(string)

			if task == "" {
				return "", fmt.Errorf("task is required")
			}
			if label == "" {
				return "", fmt.Errorf("label is required")
			}

			// Get parent session key from context if available, otherwise use default
			parentKey := "unknown"
			if key, ok := ctx.Value("session_key").(string); ok {
				parentKey = key
			}

			agentID := gw.subagentManager.Spawn(task, label, systemPrompt, parentKey, runner, gw.toolRegistry)

			return fmt.Sprintf("✅ Sub-agent spawned!\n\nID: %s\nLabel: %s\nTask: %s\n\nThe agent is now running in the background. You'll be notified when it completes.",
				agentID, label, truncateForDisplay(task, 100)), nil
		},
	})

	gw.toolRegistry.Register(&tools.Tool{
		Name:        "agent_status",
		Description: "Check status of spawned sub-agents",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to check, or omit for all agents", Required: false},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			agentID, _ := params["agent_id"].(string)

			if agentID != "" {
				agent, exists := gw.subagentManager.Get(agentID)
				if !exists {
					return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
				}
				return agent.GetStatus(), nil
			}

			agents := gw.subagentManager.List()
			if len(agents) == 0 {
				return "📭 No sub-agents have been spawned.", nil
			}

			var result string
			result = fmt.Sprintf("🤖 *Sub-agents* (%d)\n\n", len(agents))
			for _, sa := range agents {
				status, agentLabel, task, _, _ := sa.GetInfo()
				result += fmt.Sprintf("• `%s` (%s) — %s\n  Task: %s\n\n", sa.ID, agentLabel, formatSubAgentStatus(status), truncateForDisplay(task, 50))
			}
			return result, nil
		},
	})

	gw.toolRegistry.Register(&tools.Tool{
		Name:        "cancel_agent",
		Description: "Cancel a running sub-agent",
		Parameters: []tools.Parameter{
			{Name: "agent_id", Type: "string", Description: "Agent ID to cancel", Required: true},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			agentID, _ := params["agent_id"].(string)
			if agentID == "" {
				return "", fmt.Errorf("agent_id is required")
			}

			sa, exists := gw.subagentManager.Get(agentID)
			if !exists {
				return fmt.Sprintf("❌ Agent not found: %s", agentID), nil
			}

			_, agentLabel, _, _, _ := sa.GetInfo()

			if err := gw.subagentManager.Cancel(agentID); err != nil {
				return fmt.Sprintf("❌ Failed to cancel: %v", err), nil
			}

			return fmt.Sprintf("🚫 Sub-agent canceled!\n\nID: %s\nLabel: %s", agentID, agentLabel), nil
		},
	})
}

// createSubAgentChatFunc creates a function that runs agent conversations for sub-agents
func (gw *Gateway) createSubAgentChatFunc() subagent.ChatWithToolsFunc {
	return func(messages []types.Message, systemPrompt string, toolRegistry *tools.Registry, maxIterations int) (*types.AgentResponse, error) {
		gw.mu.RLock()
		router := gw.router
		gw.mu.RUnlock()

		if router == nil {
			return nil, fmt.Errorf("agent not configured")
		}

		// Get the Anthropic client
		anthropicClient, ok := router.Agent().(*agent.AnthropicClient)
		if !ok {
			return nil, fmt.Errorf("sub-agents require Anthropic provider")
		}

		// Build Anthropic tools from registry
		var anthropicTools []agent.AnthropicTool
		if toolRegistry != nil {
			for _, tool := range toolRegistry.List() {
				schema := tool.ToAnthropicSchema()
				inputSchemaBytes, _ := json.Marshal(schema["input_schema"])
				anthropicTools = append(anthropicTools, agent.AnthropicTool{
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: inputSchemaBytes,
				})
			}
		}

		// Create tool executor
		// TODO: pass parent context for graceful cancellation
		executor := func(name string, input map[string]any) (string, error) {
			if toolRegistry == nil {
				return "", fmt.Errorf("no tools available")
			}
			tool := toolRegistry.Get(name)
			if tool == nil {
				return "", fmt.Errorf("unknown tool: %s", name)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			return tool.Handler(ctx, input)
		}

		// Run the agentic loop
		return anthropicClient.ChatWithTools(messages, systemPrompt, anthropicTools, executor, maxIterations, nil)
	}
}

// handleSubAgentComplete handles a sub-agent completion
func (gw *Gateway) handleSubAgentComplete(agentID, label, result, parentSessionKey string, duration time.Duration, err error) {
	gw.log.Info("🤖 Sub-agent '%s' (%s) completed in %s", label, agentID, formatDuration(duration))

	// Format duration for display
	durationStr := formatDuration(duration)

	// Build result message
	var message string
	if err != nil {
		message = fmt.Sprintf("🤖 Sub-agent **%s** failed after %s:\n\n❌ %v", label, durationStr, err)
	} else {
		// Truncate long results for notification
		preview := result
		if len(preview) > 500 {
			preview = preview[:497] + "..."
		}
		message = fmt.Sprintf("🤖 Sub-agent **%s** completed in %s:\n\n%s", label, durationStr, preview)
	}

	// Inject result into parent session
	if parentSessionKey != "" {
		gw.injectSubAgentResult(parentSessionKey, label, result, err)
	}

	// Send Telegram notification
	gw.sendSubAgentNotification(parentSessionKey, message)
}

// injectSubAgentResult adds the sub-agent result to the parent session history
func (gw *Gateway) injectSubAgentResult(sessionKey, label, result string, err error) {
	// Parse session key to get channel and userID
	parts := parseSessionKey(sessionKey)
	if len(parts) != 2 {
		gw.log.Warn("Invalid session key for sub-agent result injection: %s", sessionKey)
		return
	}

	channel := parts[0]
	userID := parts[1]

	// Build system message content
	var content string
	if err != nil {
		content = fmt.Sprintf("[Sub-agent \"%s\" failed]\nError: %v", label, err)
	} else {
		content = fmt.Sprintf("[Sub-agent \"%s\" completed]\nResult: %s", label, result)
	}

	// Create a system-style message
	msg := types.Message{
		Text:      content,
		Channel:   channel,
		From:      "system",
		IsBot:     false, // Mark as user so it appears in context
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"subagent_result": true,
			"subagent_label":  label,
		},
	}

	// Add to session
	gw.sessions.AddMessageAndPersist(channel, userID, msg)
	gw.log.Debug("📥 Injected sub-agent result into session %s", sessionKey)
}

// sendSubAgentNotification sends a notification via Telegram
func (gw *Gateway) sendSubAgentNotification(sessionKey, message string) {
	parts := parseSessionKey(sessionKey)
	if len(parts) != 2 {
		return
	}

	channel := parts[0]
	if channel != "telegram" {
		return
	}

	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		gw.log.Warn("Invalid user ID in session key: %s", parts[1])
		return
	}

	gw.mu.RLock()
	telegram := gw.telegram
	gw.mu.RUnlock()

	if telegram != nil {
		if err := telegram.SendMessage(userID, message, true); err != nil {
			gw.log.Warn("Failed to send sub-agent notification: %v", err)
		}
	}
}

// parseSessionKey splits "channel:userID" into parts
func parseSessionKey(key string) []string {
	return strings.SplitN(key, ":", 2)
}

// subAgentPersisterAdapter wraps SQLiteStore to implement subagent.Persister
type subAgentPersisterAdapter struct {
	db *store.SQLiteStore
}

func (a *subAgentPersisterAdapter) EnsureSubAgentsTable() error {
	return a.db.EnsureSubAgentsTable()
}

func (a *subAgentPersisterAdapter) SaveSubAgent(sa *subagent.SubAgentData) error {
	return a.db.SaveSubAgent(&store.SubAgentData{
		ID:               sa.ID,
		Label:            sa.Label,
		Task:             sa.Task,
		SystemPrompt:     sa.SystemPrompt,
		Status:           sa.Status,
		Result:           sa.Result,
		Error:            sa.Error,
		StartedAt:        sa.StartedAt,
		CompletedAt:      sa.CompletedAt,
		ParentSessionKey: sa.ParentSessionKey,
	})
}

func (a *subAgentPersisterAdapter) DeleteSubAgent(id string) error {
	return a.db.DeleteSubAgent(id)
}

func (a *subAgentPersisterAdapter) LoadAllSubAgents() ([]*subagent.SubAgentData, error) {
	dbAgents, err := a.db.LoadAllSubAgents()
	if err != nil {
		return nil, err
	}
	result := make([]*subagent.SubAgentData, len(dbAgents))
	for i, sa := range dbAgents {
		result[i] = &subagent.SubAgentData{
			ID:               sa.ID,
			Label:            sa.Label,
			Task:             sa.Task,
			SystemPrompt:     sa.SystemPrompt,
			Status:           sa.Status,
			Result:           sa.Result,
			Error:            sa.Error,
			StartedAt:        sa.StartedAt,
			CompletedAt:      sa.CompletedAt,
			ParentSessionKey: sa.ParentSessionKey,
		}
	}
	return result, nil
}

func (a *subAgentPersisterAdapter) LoadPendingSubAgents() ([]*subagent.SubAgentData, error) {
	dbAgents, err := a.db.LoadPendingSubAgents()
	if err != nil {
		return nil, err
	}
	result := make([]*subagent.SubAgentData, len(dbAgents))
	for i, sa := range dbAgents {
		result[i] = &subagent.SubAgentData{
			ID:               sa.ID,
			Label:            sa.Label,
			Task:             sa.Task,
			SystemPrompt:     sa.SystemPrompt,
			Status:           sa.Status,
			Result:           sa.Result,
			Error:            sa.Error,
			StartedAt:        sa.StartedAt,
			CompletedAt:      sa.CompletedAt,
			ParentSessionKey: sa.ParentSessionKey,
		}
	}
	return result, nil
}

// truncateForDisplay truncates a string for display
func truncateForDisplay(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatSubAgentStatus returns emoji-formatted status
func formatSubAgentStatus(status string) string {
	switch status {
	case subagent.StatusPending:
		return "⏳ Pending"
	case subagent.StatusRunning:
		return "🔄 Running"
	case subagent.StatusDone:
		return "✅ Done"
	case subagent.StatusFailed:
		return "❌ Failed"
	case subagent.StatusCanceled:
		return "🚫 Canceled"
	default:
		return status
	}
}