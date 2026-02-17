package subagent

import (
	"context"
	"fmt"
	"time"

	"github.com/FeelPulse/feelpulse/internal/tools"
	"github.com/FeelPulse/feelpulse/pkg/types"
)

// ChatWithToolsFunc is the signature for running an agentic conversation
type ChatWithToolsFunc func(
	messages []types.Message,
	systemPrompt string,
	toolRegistry *tools.Registry,
	maxIterations int,
) (*types.AgentResponse, error)

// SimpleRunner implements AgentRunner using a chat function
type SimpleRunner struct {
	chatFunc      ChatWithToolsFunc
	maxIterations int
}

// NewSimpleRunner creates a new runner with a chat function
func NewSimpleRunner(chatFunc ChatWithToolsFunc, maxIterations int) *SimpleRunner {
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}
	return &SimpleRunner{
		chatFunc:      chatFunc,
		maxIterations: maxIterations,
	}
}

// RunTask implements AgentRunner
func (r *SimpleRunner) RunTask(ctx context.Context, task string, systemPrompt string, toolRegistry *tools.Registry) (string, error) {
	// Build initial message with the task
	messages := []types.Message{
		{
			Text:      task,
			IsBot:     false,
			Channel:   "subagent",
			Timestamp: time.Now(),
		},
	}

	// Create a channel for the result
	type result struct {
		resp *types.AgentResponse
		err  error
	}
	resultCh := make(chan result, 1)

	// Run the chat in a goroutine so we can respect context cancellation
	go func() {
		resp, err := r.chatFunc(messages, systemPrompt, toolRegistry, r.maxIterations)
		resultCh <- result{resp: resp, err: err}
	}()

	// Wait for result or context cancellation
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return "", fmt.Errorf("agent error: %w", res.err)
		}
		return res.resp.Text, nil
	}
}
