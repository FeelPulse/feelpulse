package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Gateway   GatewayConfig   `yaml:"gateway"`
	Agent     AgentConfig     `yaml:"agent"`
	Channels  ChannelsConfig  `yaml:"channels"`
	Hooks     HooksConfig     `yaml:"hooks"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	TTS       TTSConfig       `yaml:"tts"`
	Browser   BrowserConfig   `yaml:"browser"`
	Tools     ToolsConfig     `yaml:"tools"`
	Log       LogConfig       `yaml:"log"`
	Admin     AdminConfig     `yaml:"admin"`
	Metrics   MetricsConfig   `yaml:"metrics"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level string `yaml:"level"` // debug, info, warn, error (default: info)
}

// AdminConfig holds admin user configuration
type AdminConfig struct {
	Username string `yaml:"username"` // Admin username (defaults to first allowedUser)
}

// MetricsConfig holds metrics endpoint configuration
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable /metrics endpoint
	Path    string `yaml:"path"`    // Metrics endpoint path (default: /metrics)
}

// ToolsConfig holds tool-specific configuration
type ToolsConfig struct {
	Exec ExecToolConfig `yaml:"exec"`
	File FileToolConfig `yaml:"file"`
}

// FileToolConfig holds file tool security settings
type FileToolConfig struct {
	Enabled bool `yaml:"enabled"` // Enable file tools (default: true — safer than exec)
}

// ExecToolConfig holds exec tool security settings
type ExecToolConfig struct {
	Enabled         bool     `yaml:"enabled"`         // Enable exec tool (default: false for safety)
	AllowedCommands []string `yaml:"allowedCommands"` // Allowed command prefixes. Use ["bash"] for full access (dangerous patterns still blocked)
	TimeoutSeconds  int      `yaml:"timeoutSeconds"`  // Command timeout (default: 30)
}

type HeartbeatConfig struct {
	Enabled         bool `yaml:"enabled"`
	IntervalMinutes int  `yaml:"intervalMinutes"`
}

type TTSConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable TTS globally (default: false)
	Command string `yaml:"command"` // TTS command (espeak, say, etc.) - auto-detected if empty
}

type BrowserConfig struct {
	Enabled        bool `yaml:"enabled"`        // Enable browser tools (default: false)
	Headless       bool `yaml:"headless"`       // Run without visible window (default: true)
	TimeoutSeconds int  `yaml:"timeoutSeconds"` // Page load timeout in seconds (default: 30)
	Stealth        bool `yaml:"stealth"`        // Use stealth mode to avoid bot detection (default: true)
}

type WorkspaceConfig struct {
	Path     string            `yaml:"path"`
	Profiles map[string]string `yaml:"profiles"` // Map of profile name -> path to SOUL.md variant
}

type GatewayConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

type AgentConfig struct {
	Model            string `yaml:"model"`
	Provider         string `yaml:"provider"`
	APIKey           string `yaml:"apiKey"`
	AuthToken        string `yaml:"authToken"`        // OAuth setup-token (sk-ant-oat-...) for subscription auth
	MaxTokens        int    `yaml:"maxTokens"`
	MaxContextTokens int    `yaml:"maxContextTokens"` // Threshold for context compaction (default: 80000)
	System           string `yaml:"system"`
	FallbackModel    string `yaml:"fallbackModel"`    // Fallback model if primary fails
	FallbackProvider string `yaml:"fallbackProvider"` // Fallback provider (defaults to same as primary)
	RateLimit        int    `yaml:"rateLimit"`        // Max messages per minute per user (0 = disabled)
}

type ChannelsConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
}

type TelegramConfig struct {
	Enabled      bool     `yaml:"enabled"`
	BotToken     string   `yaml:"token"`
	AllowedUsers []string `yaml:"allowedUsers"` // empty = allow all; non-empty = only these usernames
}

type HooksConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
	Path    string `yaml:"path"`
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Gateway: GatewayConfig{
			Port: 18789,
			Bind: "localhost",
		},
		Agent: AgentConfig{
			Model:            "claude-sonnet-4-20250514",
			Provider:         "anthropic",
			MaxTokens:        4096,
			MaxContextTokens: 80000,
		},
		Channels: ChannelsConfig{
			Telegram: TelegramConfig{
				Enabled: false,
			},
		},
		Hooks: HooksConfig{
			Enabled: true,
			Path:    "/hooks",
		},
		Workspace: WorkspaceConfig{
			Path: filepath.Join(home, ".feelpulse", "workspace"),
		},
		Heartbeat: HeartbeatConfig{
			Enabled:         false,
			IntervalMinutes: 60,
		},
		TTS: TTSConfig{
			Enabled: false,
			Command: "", // Auto-detect
		},
		Browser: BrowserConfig{
			Enabled:        false, // Disabled by default (requires Chrome)
			Headless:       true,
			TimeoutSeconds: 30,
			Stealth:        true,
		},
		Tools: ToolsConfig{
			Exec: ExecToolConfig{
				Enabled:         false, // Disabled by default for security
				AllowedCommands: []string{},
				TimeoutSeconds:  30,
			},
			File: FileToolConfig{
				Enabled: true, // Safer than exec, enabled by default
			},
		},
		Log: LogConfig{
			Level: "info",
		},
		Admin: AdminConfig{
			Username: "", // Will default to first allowedUser
		},
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
	}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".feelpulse")
}

func configPath() string {
	return filepath.Join(configDir(), "config.yaml")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("config not found: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// ValidationResult holds the result of config validation
type ValidationResult struct {
	Errors   []string
	Warnings []string
}

// IsValid returns true if there are no errors
func (v *ValidationResult) IsValid() bool {
	return len(v.Errors) == 0
}

// Validate checks the configuration for required fields and common issues
func (c *Config) Validate() *ValidationResult {
	result := &ValidationResult{
		Errors:   []string{},
		Warnings: []string{},
	}

	// Check for required authentication
	if c.Agent.APIKey == "" && c.Agent.AuthToken == "" {
		result.Errors = append(result.Errors, "Agent authentication required: set agent.apiKey or agent.authToken")
	}

	// Check Telegram configuration
	if c.Channels.Telegram.Enabled {
		if c.Channels.Telegram.BotToken == "" {
			result.Errors = append(result.Errors, "Telegram enabled but token not set: set channels.telegram.token")
		}
	}

	// Check workspace path
	if c.Workspace.Path != "" {
		if _, err := os.Stat(c.Workspace.Path); os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Workspace directory does not exist: %s (run 'feelpulse workspace init')", c.Workspace.Path))
		}
	}

	// Check model
	if c.Agent.Model == "" {
		result.Warnings = append(result.Warnings, "No model specified, using default (claude-sonnet-4-20250514)")
	}

	// Check provider
	if c.Agent.Provider != "" && c.Agent.Provider != "anthropic" && c.Agent.Provider != "openai" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Unknown provider '%s', supported: anthropic, openai", c.Agent.Provider))
	}

	// Check browser dependencies
	if c.Browser.Enabled {
		result.Warnings = append(result.Warnings, "Browser tools enabled - requires Chrome/Chromium installed")
	}

	// Check heartbeat interval
	if c.Heartbeat.Enabled && c.Heartbeat.IntervalMinutes < 1 {
		result.Warnings = append(result.Warnings, "Heartbeat interval < 1 minute, may cause excessive API calls")
	}

	// Check rate limit
	if c.Agent.RateLimit > 100 {
		result.Warnings = append(result.Warnings, "Rate limit > 100 msg/min - consider lower limit for safety")
	}

	// Check profile paths
	for name, path := range c.Workspace.Profiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Profile '%s' file not found: %s", name, path))
		}
	}

	return result
}

func Save(cfg *Config) (string, error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	path := configPath()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}
