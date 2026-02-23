package command

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SkillReloadCallback is called after skill install/update to reload skills
type SkillReloadCallback func() error

// skillReloadCallback is set via SetSkillReloadCallback
var skillReloadCallback SkillReloadCallback

// SetSkillReloadCallback sets the callback for skill hot-reload
func (h *Handler) SetSkillReloadCallback(cb SkillReloadCallback) {
	skillReloadCallback = cb
}

// handleSkill processes /skill subcommands
func (h *Handler) handleSkill(args string) string {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return h.skillHelp()
	}

	subCmd := strings.ToLower(parts[0])
	subArgs := parts[1:]

	switch subCmd {
	case "list":
		return h.skillList()
	case "install":
		if len(subArgs) == 0 {
			return "❌ Usage: `/skill install <name>`\n\nExample: `/skill install github`"
		}
		return h.skillInstall(subArgs[0])
	case "update":
		return h.skillUpdate(subArgs)
	case "search":
		if len(subArgs) == 0 {
			return "❌ Usage: `/skill search <query>`\n\nExample: `/skill search github`"
		}
		return h.skillSearch(strings.Join(subArgs, " "))
	default:
		return h.skillHelp()
	}
}

// skillHelp returns help text for /skill command
func (h *Handler) skillHelp() string {
	return `🛠️ *Skill Management*

*Commands:*
  /skill list — List installed skills
  /skill install <name> — Install a skill from ClaWHub
  /skill update <name> — Update a specific skill
  /skill update --all — Update all skills
  /skill search <query> — Search for skills

*Examples:*
  /skill search "postgres"
  /skill install github
  /skill update github

_Skills are AI capabilities powered by ClawHub.com_`
}

// skillList lists installed skills from workspace/skills/
func (h *Handler) skillList() string {
	if h.memory == nil {
		return "❌ Memory manager not available."
	}

	// Read skills directory
	skillsDir := filepath.Join(h.memory.Path(), "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil || len(entries) == 0 {
		return "📭 No skills installed.\n\nUse `/skill search <query>` to find skills, then `/skill install <name>` to install."
	}

	// Collect skill names (directories only)
	var skillNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			skillNames = append(skillNames, entry.Name())
		}
	}

	if len(skillNames) == 0 {
		return "📭 No skills installed.\n\nUse `/skill search <query>` to find skills, then `/skill install <name>` to install."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🛠️ *Installed Skills* (%d)\n\n", len(skillNames)))
	for _, name := range skillNames {
		sb.WriteString(fmt.Sprintf("• %s\n", name))
	}
	sb.WriteString("\n_Use `/skill update <name>` to update a skill._")
	return sb.String()
}

// skillInstall installs a skill from ClaWHub
func (h *Handler) skillInstall(name string) string {
	workdir := h.getWorkspacePath()
	if workdir == "" {
		return "❌ Workspace path not configured."
	}

	// Check if clawhub is installed
	if !clawhubAvailable() {
		return "❌ ClawHub CLI not installed.\n\nInstall with: `npm i -g clawhub`"
	}

	// Run clawhub install
	cmd := exec.Command("clawhub", "install", name, "--workdir", workdir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Sprintf("❌ Failed to install skill `%s`:\n%s", name, errMsg)
	}

	// Hot-reload skills
	if skillReloadCallback != nil {
		if err := skillReloadCallback(); err != nil {
			return fmt.Sprintf("✅ Skill `%s` installed, but failed to reload: %v\n\nRestart gateway to use the skill.", name, err)
		}
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = "Done"
	}
	return fmt.Sprintf("✅ Skill `%s` installed!\n\n%s", name, output)
}

// skillUpdate updates skills
func (h *Handler) skillUpdate(args []string) string {
	workdir := h.getWorkspacePath()
	if workdir == "" {
		return "❌ Workspace path not configured."
	}

	if !clawhubAvailable() {
		return "❌ ClawHub CLI not installed.\n\nInstall with: `npm i -g clawhub`"
	}

	var cmdArgs []string
	if len(args) == 0 {
		return "❌ Usage: `/skill update <name>` or `/skill update --all`"
	}

	// Check for --all flag
	isAll := false
	for _, arg := range args {
		if arg == "--all" {
			isAll = true
			break
		}
	}

	if isAll {
		cmdArgs = []string{"update", "--all", "--workdir", workdir}
	} else {
		cmdArgs = []string{"update", args[0], "--workdir", workdir}
	}

	cmd := exec.Command("clawhub", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Sprintf("❌ Failed to update skills:\n%s", errMsg)
	}

	// Hot-reload skills
	if skillReloadCallback != nil {
		if err := skillReloadCallback(); err != nil {
			return fmt.Sprintf("✅ Skills updated, but failed to reload: %v\n\nRestart gateway to use the skills.", err)
		}
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = "Done"
	}

	if isAll {
		return fmt.Sprintf("✅ All skills updated!\n\n%s", output)
	}
	return fmt.Sprintf("✅ Skill `%s` updated!\n\n%s", args[0], output)
}

// skillSearch searches ClaWHub for skills
func (h *Handler) skillSearch(query string) string {
	if !clawhubAvailable() {
		return "❌ ClawHub CLI not installed.\n\nInstall with: `npm i -g clawhub`"
	}

	cmd := exec.Command("clawhub", "search", query)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return fmt.Sprintf("❌ Search failed:\n%s", errMsg)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return fmt.Sprintf("🔍 No skills found for \"%s\"", query)
	}

	return fmt.Sprintf("🔍 *Search results for \"%s\":*\n\n%s\n\n_Use `/skill install <name>` to install._", query, output)
}

// clawhubAvailable checks if clawhub CLI is installed
func clawhubAvailable() bool {
	_, err := exec.LookPath("clawhub")
	return err == nil
}

// getWorkspacePath returns the workspace path from memory manager or config
func (h *Handler) getWorkspacePath() string {
	if h.memory != nil {
		return h.memory.Path()
	}
	if h.cfg != nil && h.cfg.Workspace.Path != "" {
		return h.cfg.Workspace.Path
	}
	// Default workspace path
	return ""
}
