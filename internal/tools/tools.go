package tools

import (
	"context"
	"sync"
)

// Handler is the function signature for tool handlers
type Handler func(ctx context.Context, params map[string]any) (string, error)

// Parameter describes a tool parameter
type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, integer, number, boolean, array, object
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Tool represents a callable tool/function
type Tool struct {
	Name        string
	Description string
	Parameters  []Parameter
	Handler     Handler
}

// Registry manages available tools
type Registry struct {
	tools map[string]*Tool
	mu    sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns all registered tools
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// ToAnthropicSchema converts the tool to Anthropic's tool schema format
func (t *Tool) ToAnthropicSchema() map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0)

	for _, p := range t.Parameters {
		prop := map[string]any{
			"type": p.Type,
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		properties[p.Name] = prop

		if p.Required {
			required = append(required, p.Name)
		}
	}

	return map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"input_schema": map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}
}

// ToOpenAISchema converts the tool to OpenAI's function schema format
func (t *Tool) ToOpenAISchema() map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0)

	for _, p := range t.Parameters {
		prop := map[string]any{
			"type": p.Type,
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		properties[p.Name] = prop

		if p.Required {
			required = append(required, p.Name)
		}
	}

	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}

// GetAnthropicSchemas returns all tools in Anthropic schema format
func (r *Registry) GetAnthropicSchemas() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, t.ToAnthropicSchema())
	}
	return schemas
}

// GetOpenAISchemas returns all tools in OpenAI schema format
func (r *Registry) GetOpenAISchemas() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		schemas = append(schemas, t.ToOpenAISchema())
	}
	return schemas
}
