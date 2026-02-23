// Package skills provides a system for loading and managing AI tool skills
// from SKILL.md files in the workspace.
package skills

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/FeelPulse/feelpulse/internal/logger"
	"github.com/FeelPulse/feelpulse/internal/tools"
)

// Param represents a skill parameter
type Param struct {
	Name        string
	Type        string // string, integer, number, boolean
	Required    bool
	Description string
}

// Skill represents a loaded skill from SKILL.md
type Skill struct {
	Name        string
	Description string
	Parameters  []Param
	Executable  string // Path to run.sh or similar
	Dir         string // Directory containing the skill
}

// Loader loads skills from a directory
type Loader struct {
	dir string
}

// NewLoader creates a new skill loader for the given directory
func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

// Load scans the directory for skill subdirectories and loads them
func (l *Loader) Load() ([]*Skill, error) {
	var skills []*Skill

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No skills directory yet
		}
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(l.dir, entry.Name())
		skillPath := filepath.Join(skillDir, "SKILL.md")

		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue // Skip directories without SKILL.md
		}

		skill, err := ParseSkillMD(entry.Name(), string(content))
		if err != nil {
			logger.Warn("⚠️ Failed to parse skill %s: %v", entry.Name(), err)
			continue
		}

		skill.Dir = skillDir

		// Look for executable
		skill.Executable = findExecutable(skillDir)

		skills = append(skills, skill)
	}

	return skills, nil
}

// findExecutable looks for run.sh, run.py, run, or main executable in the skill directory
func findExecutable(dir string) string {
	candidates := []string{"run.sh", "run.py", "run", "main.sh", "main.py", "main"}

	for _, name := range candidates {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		// Check if executable (has execute permission)
		if info.Mode()&0111 != 0 {
			return path
		}
	}

	return ""
}

// paramPattern matches parameter lines like: - name (type, required): description
var paramPattern = regexp.MustCompile(`^-\s+(\w+)\s+\(([^,]+),\s*(required|optional)\):\s*(.*)$`)

// ParseSkillMD parses a SKILL.md file content into a Skill
func ParseSkillMD(name, content string) (*Skill, error) {
	skill := &Skill{
		Name:       name,
		Parameters: []Param{},
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentSection string
	var descriptionLines []string
	inDescription := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for section headers
		if strings.HasPrefix(trimmed, "## ") {
			currentSection = strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			inDescription = false
			continue
		}

		// Check for main title (# Name)
		if strings.HasPrefix(trimmed, "# ") {
			// Use filename as name, but mark we're now in description area
			inDescription = true
			continue
		}

		// Parse based on section
		switch currentSection {
		case "parameters":
			if matches := paramPattern.FindStringSubmatch(trimmed); matches != nil {
				param := Param{
					Name:        matches[1],
					Type:        strings.TrimSpace(matches[2]),
					Required:    matches[3] == "required",
					Description: strings.TrimSpace(matches[4]),
				}
				skill.Parameters = append(skill.Parameters, param)
			}
		default:
			// Collect description lines (before any section)
			if inDescription && currentSection == "" && trimmed != "" {
				descriptionLines = append(descriptionLines, trimmed)
			}
		}
	}

	if len(descriptionLines) > 0 {
		skill.Description = strings.Join(descriptionLines, " ")
	}

	return skill, nil
}

// ToTool converts a Skill to a tools.Tool for registration
func (s *Skill) ToTool() *tools.Tool {
	params := make([]tools.Parameter, len(s.Parameters))
	for i, p := range s.Parameters {
		params[i] = tools.Parameter{
			Name:        p.Name,
			Type:        p.Type,
			Description: p.Description,
			Required:    p.Required,
		}
	}

	tool := &tools.Tool{
		Name:        s.Name,
		Description: s.Description,
		Parameters:  params,
	}

	// If skill has an executable, create a handler
	if s.Executable != "" {
		tool.Handler = s.createHandler()
	}

	return tool
}

// createHandler creates a tool handler that executes the skill's script
func (s *Skill) createHandler() tools.Handler {
	return func(ctx context.Context, params map[string]any) (string, error) {
		// Build arguments from parameters
		args := make([]string, 0, len(s.Parameters))
		for _, p := range s.Parameters {
			if val, ok := params[p.Name]; ok {
				args = append(args, fmt.Sprintf("%v", val))
			} else if p.Required {
				return "", fmt.Errorf("missing required parameter: %s", p.Name)
			}
		}

		// Create command
		cmd := exec.CommandContext(ctx, s.Executable, args...)
		cmd.Dir = s.Dir

		// Set timeout
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Execute and capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("skill execution failed: %w (output: %s)", err, string(output))
		}

		return strings.TrimSpace(string(output)), nil
	}
}

// Manager manages all loaded skills and integrates with the tool registry
type Manager struct {
	dir      string
	skills   []*Skill
	registry *tools.Registry
	mu       sync.RWMutex
}

// NewManager creates a new skills manager
func NewManager(dir string) *Manager {
	mgr := &Manager{
		dir:      dir,
		registry: tools.NewRegistry(),
	}

	// Load skills on creation
	if err := mgr.Reload(); err != nil {
		logger.Warn("⚠️ Failed to load skills: %v", err)
	}

	return mgr
}

// Reload loads or reloads skills from the directory
func (m *Manager) Reload() error {
	loader := NewLoader(m.dir)
	skills, err := loader.Load()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.skills = skills

	// Register all skills as tools
	for _, skill := range skills {
		tool := skill.ToTool()
		m.registry.Register(tool)
	}

	if len(skills) > 0 {
		logger.Info("🛠️ Loaded %d skills", len(skills))
	}

	return nil
}

// ListSkills returns all loaded skills
func (m *Manager) ListSkills() []*Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Skill, len(m.skills))
	copy(result, m.skills)
	return result
}

// GetSkill retrieves a skill by name
func (m *Manager) GetSkill(name string) *Skill {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.skills {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Registry returns the tool registry with all skills registered
func (m *Manager) Registry() *tools.Registry {
	return m.registry
}

// DefaultSkillsPath returns the default path for skills (~/.feelpulse/workspace/skills)
func DefaultSkillsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".feelpulse", "workspace", "skills")
}
