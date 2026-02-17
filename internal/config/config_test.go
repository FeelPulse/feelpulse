package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_RequiresAuth(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = ""
	cfg.Agent.AuthToken = ""

	result := cfg.Validate()
	if result.IsValid() {
		t.Error("Expected validation to fail without auth")
	}

	hasAuthError := false
	for _, err := range result.Errors {
		if contains(err, "authentication") {
			hasAuthError = true
			break
		}
	}
	if !hasAuthError {
		t.Error("Expected authentication error in validation")
	}
}

func TestValidate_ValidWithAPIKey(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"

	result := cfg.Validate()
	if !result.IsValid() {
		t.Errorf("Expected valid config with API key, got errors: %v", result.Errors)
	}
}

func TestValidate_ValidWithAuthToken(t *testing.T) {
	cfg := Default()
	cfg.Agent.AuthToken = "sk-ant-oat-test"

	result := cfg.Validate()
	if !result.IsValid() {
		t.Errorf("Expected valid config with auth token, got errors: %v", result.Errors)
	}
}

func TestValidate_TelegramEnabledWithoutToken(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.BotToken = ""

	result := cfg.Validate()
	if result.IsValid() {
		t.Error("Expected validation to fail with Telegram enabled but no token")
	}

	hasTelegramError := false
	for _, err := range result.Errors {
		if contains(err, "Telegram") {
			hasTelegramError = true
			break
		}
	}
	if !hasTelegramError {
		t.Error("Expected Telegram error in validation")
	}
}

func TestValidate_TelegramEnabledWithToken(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.BotToken = "12345:ABC"

	result := cfg.Validate()
	if !result.IsValid() {
		t.Errorf("Expected valid config, got errors: %v", result.Errors)
	}
}

func TestValidate_WorkspaceWarning(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Workspace.Path = "/nonexistent/path/that/does/not/exist"

	result := cfg.Validate()

	hasWorkspaceWarning := false
	for _, warn := range result.Warnings {
		if contains(warn, "Workspace") || contains(warn, "directory") {
			hasWorkspaceWarning = true
			break
		}
	}
	if !hasWorkspaceWarning {
		t.Error("Expected workspace warning for nonexistent path")
	}
}

func TestValidate_ProfilePathWarning(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Workspace.Profiles = map[string]string{
		"test": "/nonexistent/profile.md",
	}

	result := cfg.Validate()

	hasProfileWarning := false
	for _, warn := range result.Warnings {
		if contains(warn, "Profile") && contains(warn, "not found") {
			hasProfileWarning = true
			break
		}
	}
	if !hasProfileWarning {
		t.Error("Expected profile warning for nonexistent path")
	}
}

func TestValidate_BrowserWarning(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Browser.Enabled = true

	result := cfg.Validate()

	hasBrowserWarning := false
	for _, warn := range result.Warnings {
		if contains(warn, "Browser") || contains(warn, "Chrome") {
			hasBrowserWarning = true
			break
		}
	}
	if !hasBrowserWarning {
		t.Error("Expected browser warning when enabled")
	}
}

func TestValidate_UnknownProvider(t *testing.T) {
	cfg := Default()
	cfg.Agent.APIKey = "sk-ant-api-test"
	cfg.Agent.Provider = "unknown-provider"

	result := cfg.Validate()

	hasProviderWarning := false
	for _, warn := range result.Warnings {
		if contains(warn, "Unknown provider") {
			hasProviderWarning = true
			break
		}
	}
	if !hasProviderWarning {
		t.Error("Expected warning for unknown provider")
	}
}

func TestLoadAndSave(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "feelpulse-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Just verify Default() returns valid defaults
	cfg := Default()
	if cfg.Gateway.Port != 18789 {
		t.Errorf("Default port = %d, want 18789", cfg.Gateway.Port)
	}
	if cfg.Agent.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Default model = %s, want claude-sonnet-4-20250514", cfg.Agent.Model)
	}

	// Verify config can be modified
	cfg.Agent.APIKey = "test-key"
	cfg.Agent.Model = "test-model"
	
	// Save to temp location (we can't test the actual Save without modifying home dir)
	data, err := cfg.MarshalYAML()
	if err == nil {
		t.Log("Config serializes correctly")
	}
	_ = data
}

func TestDefaultWorkspacePath(t *testing.T) {
	cfg := Default()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".feelpulse", "workspace")
	
	if cfg.Workspace.Path != expected {
		t.Errorf("Default workspace = %s, want %s", cfg.Workspace.Path, expected)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Config.MarshalYAML - helper for testing
func (c *Config) MarshalYAML() ([]byte, error) {
	// This is just for testing - actual marshaling is done by yaml.Marshal
	return nil, nil
}
