package memory

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	bootstrapFile = "BOOTSTRAP.md"
	identityFile  = "IDENTITY.md"
	soulFile      = "SOUL.md"
	userFile      = "USER.md"
	memoryFile    = "MEMORY.md"
)

// Manager handles workspace file loading and system prompt building
type Manager struct {
	path      string
	bootstrap string
	identity  string
	soul      string
	user      string
	memory    string
	skills    []skillEntry // loaded skill docs
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
	// Read BOOTSTRAP.md (first-run instructions, highest priority)
	if data, err := os.ReadFile(filepath.Join(m.path, bootstrapFile)); err == nil {
		m.bootstrap = string(data)
	} else {
		m.bootstrap = ""
	}

	// Read IDENTITY.md (bot and user identity)
	if data, err := os.ReadFile(filepath.Join(m.path, identityFile)); err == nil {
		m.identity = string(data)
	} else {
		m.identity = ""
	}

	// Read SOUL.md (persona/system prompt override)
	if data, err := os.ReadFile(filepath.Join(m.path, soulFile)); err == nil {
		m.soul = string(data)
	} else {
		m.soul = ""
	}

	// Read USER.md (user context)
	if data, err := os.ReadFile(filepath.Join(m.path, userFile)); err == nil {
		m.user = string(data)
	} else {
		m.user = ""
	}

	// Read MEMORY.md (long-term memory)
	if data, err := os.ReadFile(filepath.Join(m.path, memoryFile)); err == nil {
		m.memory = string(data)
	} else {
		m.memory = ""
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
// Priority: BOOTSTRAP.md > SOUL.md > defaultPrompt
// Then appends: IDENTITY, USER, MEMORY, Workspace, Skills
func (m *Manager) BuildSystemPrompt(defaultPrompt string) string {
	var parts []string

	// BOOTSTRAP.md has highest priority (first-run instructions)
	if m.bootstrap != "" {
		parts = append(parts, m.bootstrap)
	} else if m.soul != "" {
		// SOUL.md overrides default prompt
		parts = append(parts, m.soul)
	} else if defaultPrompt != "" {
		// Use default prompt as fallback
		parts = append(parts, defaultPrompt)
	}

	// Append IDENTITY.md (bot and user identity)
	if m.identity != "" {
		parts = append(parts, "\n\n## Identity\n"+m.identity)
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

// Reborn resets the workspace to initial state by recreating BOOTSTRAP.md
// and removing IDENTITY.md. Returns the path to the created BOOTSTRAP.md.
func (m *Manager) Reborn() (string, error) {
	bootstrapPath := filepath.Join(m.path, bootstrapFile)
	identityPath := filepath.Join(m.path, identityFile)

	// Remove IDENTITY.md if it exists
	_ = os.Remove(identityPath)

	// Recreate BOOTSTRAP.md
	bootstrapContent := `# Bootstrap - First Steps

This is a fresh start. Before doing anything else:

1. Greet the user warmly and introduce yourself as an AI assistant
2. Ask the user what their name is
3. Ask the user what they would like to call you (your name/identity)
4. Write your identity information to IDENTITY.md in this format:

` + "```" + `
# Identity

- My name: <name you chose>
- User's name: <their name>
- Created: <current date in YYYY-MM-DD format>
` + "```" + `

5. Delete this BOOTSTRAP.md file using file_write with empty content or exec rm
6. Thank the user warmly and ask how you can help them today

Important: Complete all steps above in your first interaction. Be warm and personable!
`

	if err := os.WriteFile(bootstrapPath, []byte(bootstrapContent), 0644); err != nil {
		return "", err
	}

	// Reload to pick up the new BOOTSTRAP.md
	m.Load()

	return bootstrapPath, nil
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
		bootstrapFile: `# Bootstrap - First Steps

This is your first time running. Before doing anything else:

1. Greet the user warmly and introduce yourself as an AI assistant
2. Ask the user what their name is
3. Ask the user what they would like to call you (your name/identity)
4. Write your identity information to IDENTITY.md in this format:

` + "```" + `
# Identity

- My name: <name you chose>
- User's name: <their name>  
- Created: <current date in YYYY-MM-DD format>
` + "```" + `

5. Delete this BOOTSTRAP.md file using file_write with empty content or exec rm
6. Thank the user warmly and ask how you can help them today

Important: Complete all steps above in your first interaction. Be warm and personable!
`,
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
