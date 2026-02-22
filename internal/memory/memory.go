package memory

import (
	"embed"
	"fmt"
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

// skillEntry holds a loaded skill's name and description
type skillEntry struct {
	Name        string
	Description string // first non-empty, non-heading line from SKILL.md
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
				desc := extractSkillDescription(string(data))
				m.skills = append(m.skills, skillEntry{
					Name:        entry.Name(),
					Description: desc,
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

	// Workspace section
	parts = append(parts, fmt.Sprintf(`

## Workspace

Working directory: %s

All file operations (file_read, file_write, file_list) are sandboxed to this directory.
Always clone git repos here: git clone <url> %s/<repo-name>`, m.path, m.path))

	// Available skills section
	if len(m.skills) > 0 {
		skillsList := "\n\n## Available Skills\n\nThe following skills are installed. Use `read_skill` to load full documentation on demand:\n\n"
		for _, s := range m.skills {
			skillsList += fmt.Sprintf("- **%s**: %s\n", s.Name, s.Description)
		}
		parts = append(parts, skillsList)
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

// stripFrontmatter removes YAML frontmatter from content and returns the body.
// Frontmatter is delimited by --- at the start and end.
func stripFrontmatter(content string) (body string, frontmatter string) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content, ""
	}

	// Find closing ---
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			// Found end of frontmatter
			frontmatter = strings.Join(lines[1:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return strings.TrimLeft(body, "\n"), frontmatter
		}
	}

	// No closing ---, return original
	return content, ""
}

// extractFrontmatterField extracts a field value from YAML frontmatter.
// Handles simple key: "value" or key: value patterns.
func extractFrontmatterField(frontmatter, field string) string {
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		prefix := field + ":"
		if strings.HasPrefix(trimmed, prefix) {
			value := strings.TrimSpace(trimmed[len(prefix):])
			// Remove surrounding quotes
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				return value[1 : len(value)-1]
			}
			if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
				return value[1 : len(value)-1]
			}
			return value
		}
	}
	return ""
}

// extractSkillDescription extracts the description from SKILL.md content.
// First checks YAML frontmatter for description field, then falls back to first text line.
func extractSkillDescription(content string) string {
	body, frontmatter := stripFrontmatter(content)

	// Try to extract from frontmatter first
	if frontmatter != "" {
		if desc := extractFrontmatterField(frontmatter, "description"); desc != "" {
			return desc
		}
	}

	// Fall back to first non-empty, non-heading line from body
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

// ReadSkill reads the full SKILL.md content for a skill by name.
// Frontmatter is stripped before returning.
func (m *Manager) ReadSkill(name string) (string, error) {
	skillPath := filepath.Join(m.path, "skills", name, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("skill '%s' not found", name)
	}
	body, _ := stripFrontmatter(string(data))
	return body, nil
}

// Path returns the workspace path
func (m *Manager) Path() string {
	return m.path
}

// ListSkillNames returns the names of all loaded skills
func (m *Manager) ListSkillNames() []string {
	names := make([]string, len(m.skills))
	for i, s := range m.skills {
		names[i] = s.Name
	}
	return names
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
