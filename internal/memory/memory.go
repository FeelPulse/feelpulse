package memory

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
)

const (
	soulFile   = "SOUL.md"
	userFile   = "USER.md"
	memoryFile = "MEMORY.md"
)

// Manager handles workspace file loading and system prompt building
type Manager struct {
	path   string
	soul   string
	user   string
	memory string
	skills []skillEntry // loaded skill docs
}

// skillEntry holds a loaded skill's name and content
type skillEntry struct {
	Name    string
	Content string
}

// NewManager creates a new workspace Manager for the given path
func NewManager(workspacePath string) *Manager {
	return &Manager{path: workspacePath}
}

// Load reads workspace files from disk. Missing files are silently ignored.
func (m *Manager) Load() error {
	// Read SOUL.md (persona/system prompt override)
	if data, err := os.ReadFile(filepath.Join(m.path, soulFile)); err == nil {
		m.soul = string(data)
	}

	// Read USER.md (user context)
	if data, err := os.ReadFile(filepath.Join(m.path, userFile)); err == nil {
		m.user = string(data)
	}

	// Read MEMORY.md (long-term memory)
	if data, err := os.ReadFile(filepath.Join(m.path, memoryFile)); err == nil {
		m.memory = string(data)
	}

	// Load skills from skills/ directory
	m.skills = nil
	skillsDir := filepath.Join(m.path, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			if data, err := os.ReadFile(skillPath); err == nil {
				m.skills = append(m.skills, skillEntry{
					Name:    entry.Name(),
					Content: string(data),
				})
			}
		}
	}

	return nil
}

// Soul returns the loaded SOUL.md content
func (m *Manager) Soul() string {
	return m.soul
}

// User returns the loaded USER.md content
func (m *Manager) User() string {
	return m.user
}

// Memory returns the loaded MEMORY.md content
func (m *Manager) Memory() string {
	return m.memory
}

// BuildSystemPrompt constructs the full system prompt by combining workspace files.
// If SOUL.md exists, it replaces/prepends the default prompt.
// USER.md and MEMORY.md are appended as context sections.
func (m *Manager) BuildSystemPrompt(defaultPrompt string) string {
	var parts []string

	// SOUL.md replaces/prepends the base system prompt
	if m.soul != "" {
		parts = append(parts, m.soul)
	} else if defaultPrompt != "" {
		parts = append(parts, defaultPrompt)
	}

	// Append USER.md as user context section
	if m.user != "" {
		parts = append(parts, "\n\n## User Context\n"+m.user)
	}

	// Append MEMORY.md as memory section
	if m.memory != "" {
		parts = append(parts, "\n\n## Memory\n"+m.memory)
	}

	// Append skills as reference documentation
	if len(m.skills) > 0 {
		var skillParts []string
		for _, s := range m.skills {
			skillParts = append(skillParts, "### Skill: "+s.Name+"\n"+s.Content)
		}
		parts = append(parts, "\n\n## Available Skills\n\nUse these skills via the exec tool when the relevant CLI is available.\n\n"+strings.Join(skillParts, "\n\n---\n\n"))
	}

	result := strings.Join(parts, "")
	// If nothing was added, return default
	if result == "" {
		return defaultPrompt
	}
	return result
}

// DefaultWorkspacePath returns the default workspace path (~/.feelpulse/workspace)
func DefaultWorkspacePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".feelpulse", "workspace")
}

// InitWorkspace creates the workspace directory and template files.
// Does NOT overwrite existing files.
func InitWorkspace(workspacePath string) error {
	// Create directory
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return err
	}

	// Template content for each file
	templates := map[string]string{
		soulFile: `# Soul - Your AI Persona

This file defines who you are. Write in first person.

Example:
You are FeelPulse, a friendly and helpful AI assistant. You're concise, thoughtful, and occasionally witty.
`,
		userFile: `# User Context

Information about the user that should inform your responses.

Example:
- Name: (user's name)
- Preferences: (communication style, topics of interest)
- Timezone: (user's timezone)
`,
		memoryFile: `# Long-term Memory

Important things to remember across conversations.

Example:
- User mentioned they work in software engineering
- User prefers technical explanations over simplified ones
- Last discussed project: (project name)
`,
	}

	// Write template files (skip if already exists)
	for filename, content := range templates {
		path := filepath.Join(workspacePath, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	// Create skills directory with bundled skills
	skillsDir := filepath.Join(workspacePath, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return err
	}

	// Install bundled skills (skip if already exists)
	for name, content := range getBundledSkills() {
		dir := filepath.Join(skillsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		path := filepath.Join(dir, "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

// bundledSkills contains built-in skills that ship with FeelPulse
//
//go:embed skills
var skillsFS embed.FS

// getBundledSkills reads embedded skill files
func getBundledSkills() map[string]string {
	result := make(map[string]string)
	entries, err := skillsFS.ReadDir("skills")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := skillsFS.ReadFile("skills/" + entry.Name() + "/SKILL.md")
		if err != nil {
			continue
		}
		result[entry.Name()] = string(data)
	}
	return result
}
