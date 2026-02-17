package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillMD(t *testing.T) {
	content := `# Weather Skill

Get current weather for any location.

## Parameters

- location (string, required): City name or coordinates
- units (string, optional): "metric" or "imperial", default: metric

## Example

"What's the weather in Paris?"
`
	skill, err := ParseSkillMD("weather", content)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if skill.Name != "weather" {
		t.Errorf("expected name 'weather', got '%s'", skill.Name)
	}

	if skill.Description != "Get current weather for any location." {
		t.Errorf("unexpected description: '%s'", skill.Description)
	}

	if len(skill.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(skill.Parameters))
	}

	// Check location param
	loc := skill.Parameters[0]
	if loc.Name != "location" || loc.Type != "string" || !loc.Required {
		t.Errorf("location param incorrect: %+v", loc)
	}

	// Check units param
	units := skill.Parameters[1]
	if units.Name != "units" || units.Type != "string" || units.Required {
		t.Errorf("units param incorrect: %+v", units)
	}
}

func TestParseSkillMD_MinimalFormat(t *testing.T) {
	content := `# Calculator

Perform arithmetic calculations.
`
	skill, err := ParseSkillMD("calculator", content)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if skill.Name != "calculator" {
		t.Errorf("expected name 'calculator', got '%s'", skill.Name)
	}

	if skill.Description != "Perform arithmetic calculations." {
		t.Errorf("unexpected description: '%s'", skill.Description)
	}

	if len(skill.Parameters) != 0 {
		t.Errorf("expected 0 parameters, got %d", len(skill.Parameters))
	}
}

func TestParseSkillMD_ParamTypes(t *testing.T) {
	content := `# Test Skill

Test various parameter types.

## Parameters

- count (integer, required): Number of items
- enabled (boolean, optional): Enable feature
- ratio (number, optional): Decimal ratio
`
	skill, err := ParseSkillMD("test", content)
	if err != nil {
		t.Fatalf("ParseSkillMD failed: %v", err)
	}

	if len(skill.Parameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(skill.Parameters))
	}

	if skill.Parameters[0].Type != "integer" {
		t.Errorf("expected count type 'integer', got '%s'", skill.Parameters[0].Type)
	}

	if skill.Parameters[1].Type != "boolean" {
		t.Errorf("expected enabled type 'boolean', got '%s'", skill.Parameters[1].Type)
	}

	if skill.Parameters[2].Type != "number" {
		t.Errorf("expected ratio type 'number', got '%s'", skill.Parameters[2].Type)
	}
}

func TestLoaderLoadFromDirectory(t *testing.T) {
	// Create temp directory with test skills
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create skill directory
	skillDir := filepath.Join(tmpDir, "weather")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `# Weather

Get current weather information.

## Parameters

- city (string, required): City name
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create loader and load skills
	loader := NewLoader(tmpDir)
	skills, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "weather" {
		t.Errorf("expected skill name 'weather', got '%s'", skills[0].Name)
	}
}

func TestLoaderLoadWithExecutable(t *testing.T) {
	// Create temp directory with skill + executable
	tmpDir, err := os.MkdirTemp("", "skills-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create skill directory
	skillDir := filepath.Join(tmpDir, "echo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Write SKILL.md
	skillContent := `# Echo

Echo the input message.

## Parameters

- message (string, required): Message to echo
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Write executable run.sh
	runScript := `#!/bin/sh
echo "Echo: $1"
`
	runPath := filepath.Join(skillDir, "run.sh")
	if err := os.WriteFile(runPath, []byte(runScript), 0755); err != nil {
		t.Fatalf("Failed to write run.sh: %v", err)
	}

	// Load skills
	loader := NewLoader(tmpDir)
	skills, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Executable == "" {
		t.Error("expected executable path to be set")
	}

	if skills[0].Executable != runPath {
		t.Errorf("expected executable '%s', got '%s'", runPath, skills[0].Executable)
	}
}

func TestSkillToTool(t *testing.T) {
	skill := &Skill{
		Name:        "test",
		Description: "Test skill",
		Parameters: []Param{
			{Name: "arg1", Type: "string", Required: true, Description: "First argument"},
		},
		Executable: "/path/to/run.sh",
	}

	tool := skill.ToTool()

	if tool.Name != "test" {
		t.Errorf("expected tool name 'test', got '%s'", tool.Name)
	}

	if tool.Description != "Test skill" {
		t.Errorf("unexpected description: '%s'", tool.Description)
	}

	if len(tool.Parameters) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(tool.Parameters))
	}

	if tool.Parameters[0].Name != "arg1" {
		t.Errorf("expected param name 'arg1', got '%s'", tool.Parameters[0].Name)
	}

	if tool.Handler == nil {
		t.Error("expected handler to be set")
	}
}

func TestSkillHandlerExecution(t *testing.T) {
	// Create temp dir with executable
	tmpDir, err := os.MkdirTemp("", "skills-exec-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test script that outputs JSON
	runPath := filepath.Join(tmpDir, "run.sh")
	script := `#!/bin/sh
echo "Hello, $1!"
`
	if err := os.WriteFile(runPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to write script: %v", err)
	}

	skill := &Skill{
		Name:        "greet",
		Description: "Greet someone",
		Parameters: []Param{
			{Name: "name", Type: "string", Required: true},
		},
		Executable: runPath,
	}

	tool := skill.ToTool()

	// Execute the handler
	result, err := tool.Handler(context.Background(), map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("Handler execution failed: %v", err)
	}

	if !strings.Contains(result, "Hello, World!") {
		t.Errorf("unexpected result: '%s'", result)
	}
}

func TestManagerRegisterSkills(t *testing.T) {
	// Create temp directory with a skill
	tmpDir, err := os.MkdirTemp("", "skills-manager-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create skill
	skillDir := filepath.Join(tmpDir, "hello")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	skillContent := `# Hello

Say hello.

## Parameters

- name (string, required): Name to greet
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to write SKILL.md: %v", err)
	}

	// Create manager
	mgr := NewManager(tmpDir)

	// Load and check skills
	skills := mgr.ListSkills()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "hello" {
		t.Errorf("expected skill name 'hello', got '%s'", skills[0].Name)
	}
}

func TestManagerListSkills(t *testing.T) {
	// Create temp directory with multiple skills
	tmpDir, err := os.MkdirTemp("", "skills-list-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two skills
	for _, name := range []string{"alpha", "beta"} {
		skillDir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}

		content := "# " + name + "\n\nDescription for " + name + "."
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
	}

	mgr := NewManager(tmpDir)
	skills := mgr.ListSkills()

	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Check names (order may vary)
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}

	if !names["alpha"] || !names["beta"] {
		t.Errorf("expected alpha and beta skills, got: %v", names)
	}
}
