package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/internal/tools"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// SubAgentData represents a sub-agent for storage (interface type to avoid import cycles)
type SubAgentData struct {
	ID               string
	Label            string
	Task             string
	SystemPrompt     string
	Status           string
	Result           string
	Error            string
	StartedAt        time.Time
	CompletedAt      time.Time
	ParentSessionKey string
}

// Persister is the interface for sub-agent persistence
type Persister interface {
	SaveSubAgent(sa *SubAgentData) error
	DeleteSubAgent(id string) error
	LoadAllSubAgents() ([]*SubAgentData, error)
	LoadPendingSubAgents() ([]*SubAgentData, error)
	EnsureSubAgentsTable() error
}

const (
	StatusPending  = "pending"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"

	DefaultMaxRuntime    = 10 * time.Minute
	DefaultMaxIterations = 20
)

// SubAgent represents an isolated agent session running in the background
type SubAgent struct {
	ID               string
	Label            string
	Task             string
	SystemPrompt     string
	Status           string
	Result           string
	Error            string
	StartedAt        time.Time
	CompletedAt      time.Time
	Messages         []types.Message
	ParentSessionKey string

	cancel context.CancelFunc
	mu     sync.RWMutex
}

// GetInfo returns a thread-safe snapshot of agent info
func (a *SubAgent) GetInfo() (status, label, task, result, errMsg string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status, a.Label, a.Task, a.Result, a.Error
}

// AgentRunner is the interface for running AI agent conversations
type AgentRunner interface {
	// RunTask runs a task and returns the result
	// It receives messages and system prompt, and returns the final response
	RunTask(ctx context.Context, task string, systemPrompt string, toolRegistry *tools.Registry) (string, error)
}

// OnCompleteFunc is called when a sub-agent completes
// duration is the execution time of the sub-agent
type OnCompleteFunc func(agentID, label, result, parentSessionKey string, duration time.Duration, err error)

// Manager spawns and tracks sub-agents
type Manager struct {
	agents        map[string]*SubAgent
	mu            sync.RWMutex
	onComplete    OnCompleteFunc
	maxRuntime    time.Duration
	maxIterations int
	persister     Persister
}

// NewManager creates a new sub-agent manager
func NewManager(onComplete OnCompleteFunc) *Manager {
	return &Manager{
		agents:        make(map[string]*SubAgent),
		onComplete:    onComplete,
		maxRuntime:    DefaultMaxRuntime,
		maxIterations: DefaultMaxIterations,
	}
}

// SetMaxRuntime sets the maximum runtime for sub-agents
func (m *Manager) SetMaxRuntime(d time.Duration) {
	m.maxRuntime = d
}

// SetMaxIterations sets the maximum number of tool iterations
func (m *Manager) SetMaxIterations(n int) {
	m.maxIterations = n
}

// SetPersister sets the persistence backend and loads existing sub-agents
func (m *Manager) SetPersister(p Persister) error {
	if p == nil {
		return nil
	}

	// Create table if needed
	if err := p.EnsureSubAgentsTable(); err != nil {
		return fmt.Errorf("failed to create sub_agents table: %w", err)
	}

	m.mu.Lock()
	m.persister = p
	m.mu.Unlock()

	// Load all completed sub-agents for history
	agents, err := p.LoadAllSubAgents()
	if err != nil {
		return fmt.Errorf("failed to load sub-agents: %w", err)
	}

	m.mu.Lock()
	for _, sa := range agents {
		m.agents[sa.ID] = &SubAgent{
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
	m.mu.Unlock()

	return nil
}

// persist saves a sub-agent to the database
func (m *Manager) persist(agent *SubAgent) {
	m.mu.RLock()
	persister := m.persister
	m.mu.RUnlock()

	if persister == nil {
		return
	}

	agent.mu.RLock()
	data := &SubAgentData{
		ID:               agent.ID,
		Label:            agent.Label,
		Task:             agent.Task,
		SystemPrompt:     agent.SystemPrompt,
		Status:           agent.Status,
		Result:           agent.Result,
		Error:            agent.Error,
		StartedAt:        agent.StartedAt,
		CompletedAt:      agent.CompletedAt,
		ParentSessionKey: agent.ParentSessionKey,
	}
	agent.mu.RUnlock()

	_ = persister.SaveSubAgent(data)
}

// generateID creates a unique agent ID
func generateID() string {
	return fmt.Sprintf("sa-%d", time.Now().UnixNano()%1000000)
}

// Spawn creates and starts a new sub-agent
// It returns the agent ID immediately; the agent runs in the background
func (m *Manager) Spawn(
	task, label, systemPrompt, parentSessionKey string,
	runner AgentRunner,
	toolRegistry *tools.Registry,
) string {
	id := generateID()

	// Create sub-agent
	agent := &SubAgent{
		ID:               id,
		Label:            label,
		Task:             task,
		SystemPrompt:     systemPrompt,
		Status:           StatusPending,
		StartedAt:        time.Now(),
		ParentSessionKey: parentSessionKey,
		Messages:         make([]types.Message, 0),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), m.maxRuntime)
	agent.cancel = cancel

	// Register the agent
	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	// Persist initial state
	m.persist(agent)

	// Filter tools: remove spawn_agent to prevent recursive spawning
	filteredRegistry := filterTools(toolRegistry)

	// Run the agent in a goroutine
	go m.runAgent(ctx, agent, runner, filteredRegistry)

	return id
}

// filterTools creates a new registry without the spawn_agent tool
func filterTools(registry *tools.Registry) *tools.Registry {
	if registry == nil {
		return nil
	}

	filtered := tools.NewRegistry()
	for _, tool := range registry.List() {
		// Skip spawn_agent to prevent recursive spawning
		if tool.Name == "spawn_agent" {
			continue
		}
		filtered.Register(tool)
	}
	return filtered
}

// runAgent executes the agent task
func (m *Manager) runAgent(ctx context.Context, agent *SubAgent, runner AgentRunner, toolRegistry *tools.Registry) {
	// Update status to running
	agent.mu.Lock()
	agent.Status = StatusRunning
	startTime := agent.StartedAt
	agent.mu.Unlock()

	// Persist running status
	m.persist(agent)

	// Default system prompt if not provided
	systemPrompt := agent.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = fmt.Sprintf(`You are a sub-agent working on a specific task.
Your task: %s

Complete this task and provide a clear, concise result.
Focus only on the task. Do not engage in conversation.
When done, provide your final answer/result.`, agent.Task)
	}

	// Run the task
	result, err := runner.RunTask(ctx, agent.Task, systemPrompt, toolRegistry)

	// Update agent state
	agent.mu.Lock()
	agent.CompletedAt = time.Now()
	duration := agent.CompletedAt.Sub(startTime)
	if ctx.Err() == context.Canceled {
		agent.Status = StatusCanceled
		agent.Error = "canceled by user"
	} else if ctx.Err() == context.DeadlineExceeded {
		agent.Status = StatusFailed
		agent.Error = "timeout exceeded"
	} else if err != nil {
		agent.Status = StatusFailed
		agent.Error = err.Error()
	} else {
		agent.Status = StatusDone
		agent.Result = result
	}
	status := agent.Status
	label := agent.Label
	parentKey := agent.ParentSessionKey
	finalResult := agent.Result
	finalErr := agent.Error
	agent.mu.Unlock()

	// Persist completed state
	m.persist(agent)

	// Call completion callback with duration
	if m.onComplete != nil {
		var callbackErr error
		if status == StatusFailed || status == StatusCanceled {
			callbackErr = fmt.Errorf("%s", finalErr)
		}
		m.onComplete(agent.ID, label, finalResult, parentKey, duration, callbackErr)
	}
}

// Get retrieves a sub-agent by ID
func (m *Manager) Get(id string) (*SubAgent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[id]
	return agent, exists
}

// List returns all sub-agents (optionally filtered by status)
func (m *Manager) List(statusFilter ...string) []*SubAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SubAgent, 0, len(m.agents))
	for _, agent := range m.agents {
		// If filter provided, check status
		if len(statusFilter) > 0 {
			agent.mu.RLock()
			status := agent.Status
			agent.mu.RUnlock()

			match := false
			for _, f := range statusFilter {
				if status == f {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		result = append(result, agent)
	}

	return result
}

// Cancel stops a running sub-agent
func (m *Manager) Cancel(id string) error {
	m.mu.RLock()
	agent, exists := m.agents[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	if agent.Status != StatusRunning && agent.Status != StatusPending {
		return fmt.Errorf("agent is not running (status: %s)", agent.Status)
	}

	if agent.cancel != nil {
		agent.cancel()
	}
	agent.Status = StatusCanceled
	agent.CompletedAt = time.Now()
	agent.Error = "canceled by user"

	return nil
}

// Cleanup removes completed sub-agents older than maxAge
func (m *Manager) Cleanup(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for id, agent := range m.agents {
		agent.mu.RLock()
		status := agent.Status
		completedAt := agent.CompletedAt
		agent.mu.RUnlock()

		// Only cleanup completed/failed/canceled agents
		if status == StatusDone || status == StatusFailed || status == StatusCanceled {
			if !completedAt.IsZero() && now.Sub(completedAt) > maxAge {
				delete(m.agents, id)
				removed++
			}
		}
	}

	return removed
}

// GetStatus returns a formatted status for a sub-agent
func (a *SubAgent) GetStatus() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🤖 *Sub-agent: %s* (`%s`)\n", a.Label, a.ID))
	sb.WriteString(fmt.Sprintf("📋 Task: %s\n", truncate(a.Task, 100)))
	sb.WriteString(fmt.Sprintf("📊 Status: %s\n", formatStatus(a.Status)))
	sb.WriteString(fmt.Sprintf("⏱ Started: %s\n", a.StartedAt.Format(time.Kitchen)))

	if !a.CompletedAt.IsZero() {
		duration := a.CompletedAt.Sub(a.StartedAt)
		sb.WriteString(fmt.Sprintf("✅ Completed: %s (took %s)\n", a.CompletedAt.Format(time.Kitchen), formatDuration(duration)))
	} else if a.Status == StatusRunning {
		// Show elapsed time for running agents
		elapsed := time.Since(a.StartedAt)
		sb.WriteString(fmt.Sprintf("⏳ Running for: %s\n", formatDuration(elapsed)))
	}

	if a.Status == StatusDone && a.Result != "" {
		sb.WriteString(fmt.Sprintf("\n📝 *Result:*\n%s", a.Result))
	}

	if a.Error != "" {
		sb.WriteString(fmt.Sprintf("\n❌ *Error:* %s", a.Error))
	}

	return sb.String()
}

// GetDuration returns the duration of the sub-agent execution
func (a *SubAgent) GetDuration() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.CompletedAt.IsZero() {
		return a.CompletedAt.Sub(a.StartedAt)
	}
	return time.Since(a.StartedAt)
}

// formatStatus returns an emoji-formatted status
func formatStatus(status string) string {
	switch status {
	case StatusPending:
		return "⏳ Pending"
	case StatusRunning:
		return "🔄 Running"
	case StatusDone:
		return "✅ Done"
	case StatusFailed:
		return "❌ Failed"
	case StatusCanceled:
		return "🚫 Canceled"
	default:
		return status
	}
}

// formatDuration formats a duration nicely
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// truncate truncates a string to maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
