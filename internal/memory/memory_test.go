package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_LoadWorkspaceFiles(t *testing.T) {
	// Create temp workspace dir
	tmpDir := t.TempDir()

	// Write test workspace files
	soulContent := "You are a helpful assistant named TestBot."
	userContent := "# User Info\nName: TestUser\nPreferences: Brief responses"
	memoryContent := "# Long-term Memory\n- User likes cats\n- User works remotely"

	if err := os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte(soulContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "USER.md"), []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(memoryContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create manager and load
	mgr := NewManager(tmpDir)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded content
	if mgr.Soul() != soulContent {
		t.Errorf("Soul mismatch: got %q, want %q", mgr.Soul(), soulContent)
	}
	if mgr.User() != userContent {
		t.Errorf("User mismatch: got %q, want %q", mgr.User(), userContent)
	}
	if mgr.Memory() != memoryContent {
		t.Errorf("Memory mismatch: got %q, want %q", mgr.Memory(), memoryContent)
	}
}

func TestManager_MissingFiles_Graceful(t *testing.T) {
	// Empty temp dir - no workspace files
	tmpDir := t.TempDir()

	mgr := NewManager(tmpDir)
	// Should NOT fail when files don't exist
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load should handle missing files gracefully, got: %v", err)
	}

	// All should be empty
	if mgr.Soul() != "" {
		t.Errorf("Soul should be empty for missing file, got: %q", mgr.Soul())
	}
	if mgr.User() != "" {
		t.Errorf("User should be empty for missing file, got: %q", mgr.User())
	}
	if mgr.Memory() != "" {
		t.Errorf("Memory should be empty for missing file, got: %q", mgr.Memory())
	}
}

func TestManager_PartialFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Only SOUL.md exists
	soulContent := "Custom persona only"
	if err := os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte(soulContent), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(tmpDir)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mgr.Soul() != soulContent {
		t.Errorf("Soul mismatch: got %q, want %q", mgr.Soul(), soulContent)
	}
	if mgr.User() != "" {
		t.Errorf("User should be empty, got: %q", mgr.User())
	}
}

func TestManager_BuildSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()

	soulContent := "You are FeelPulse, a friendly AI."
	userContent := "User is named Alice."
	memoryContent := "Alice likes hiking."

	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte(soulContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "USER.md"), []byte(userContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(memoryContent), 0644)

	mgr := NewManager(tmpDir)
	mgr.Load()

	defaultPrompt := "Default system prompt here."
	result := mgr.BuildSystemPrompt(defaultPrompt)

	// SOUL.md should replace/prepend the system prompt
	if !strings.HasPrefix(result, soulContent) {
		t.Errorf("Result should start with SOUL.md content, got: %s", result)
	}

	// USER.md and MEMORY.md should be appended
	if !strings.Contains(result, "## User Context") {
		t.Errorf("Result should contain User Context section")
	}
	if !strings.Contains(result, userContent) {
		t.Errorf("Result should contain USER.md content")
	}
	if !strings.Contains(result, "## Memory") {
		t.Errorf("Result should contain Memory section")
	}
	if !strings.Contains(result, memoryContent) {
		t.Errorf("Result should contain MEMORY.md content")
	}
}

func TestManager_BuildSystemPrompt_NoSoul_UsesDefault(t *testing.T) {
	tmpDir := t.TempDir()

	// No SOUL.md, only USER.md
	userContent := "User context here."
	os.WriteFile(filepath.Join(tmpDir, "USER.md"), []byte(userContent), 0644)

	mgr := NewManager(tmpDir)
	mgr.Load()

	defaultPrompt := "Default system prompt."
	result := mgr.BuildSystemPrompt(defaultPrompt)

	// Should use default prompt when no SOUL.md
	if !strings.HasPrefix(result, defaultPrompt) {
		t.Errorf("Result should start with default prompt when no SOUL.md, got: %s", result)
	}
	if !strings.Contains(result, userContent) {
		t.Errorf("Result should still contain USER.md content")
	}
}

func TestManager_BuildSystemPrompt_Empty(t *testing.T) {
	tmpDir := t.TempDir()

	mgr := NewManager(tmpDir)
	mgr.Load()

	defaultPrompt := "Default prompt."
	result := mgr.BuildSystemPrompt(defaultPrompt)

	// Should contain the default prompt and workspace path injection
	if !strings.Contains(result, defaultPrompt) {
		t.Errorf("Result should contain default prompt. Got: %q", result)
	}
	if !strings.Contains(result, "Workspace path:") {
		t.Errorf("Result should contain workspace path. Got: %q", result)
	}
}

func TestDefaultWorkspacePath(t *testing.T) {
	path := DefaultWorkspacePath()
	if !strings.Contains(path, ".feelpulse") || !strings.HasSuffix(path, "workspace") {
		t.Errorf("DefaultWorkspacePath should be ~/.feelpulse/workspace, got: %s", path)
	}
}

func TestInitWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")

	if err := InitWorkspace(workspacePath); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}

	// Check directory was created
	info, err := os.Stat(workspacePath)
	if err != nil {
		t.Fatalf("Workspace dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Workspace path is not a directory")
	}

	// Check template files exist
	files := []string{"SOUL.md", "USER.md", "MEMORY.md"}
	for _, f := range files {
		path := filepath.Join(workspacePath, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Template file %s not created", f)
		}
	}

	// Verify template content is not empty
	soulData, _ := os.ReadFile(filepath.Join(workspacePath, "SOUL.md"))
	if len(soulData) == 0 {
		t.Error("SOUL.md template should have default content")
	}
}

func TestInitWorkspace_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	workspacePath := filepath.Join(tmpDir, "workspace")

	// First init
	if err := InitWorkspace(workspacePath); err != nil {
		t.Fatalf("First InitWorkspace failed: %v", err)
	}

	// Modify a file
	customContent := "Custom user content"
	os.WriteFile(filepath.Join(workspacePath, "USER.md"), []byte(customContent), 0644)

	// Second init should NOT overwrite existing files
	if err := InitWorkspace(workspacePath); err != nil {
		t.Fatalf("Second InitWorkspace failed: %v", err)
	}

	// Verify custom content preserved
	data, _ := os.ReadFile(filepath.Join(workspacePath, "USER.md"))
	if string(data) != customContent {
		t.Errorf("InitWorkspace should not overwrite existing files. Got: %q, want: %q", string(data), customContent)
	}
}
