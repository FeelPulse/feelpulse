package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Gateway  GatewayConfig  `yaml:"gateway"`
	Agent    AgentConfig    `yaml:"agent"`
	Channels ChannelsConfig `yaml:"channels"`
	Hooks    HooksConfig    `yaml:"hooks"`
}

type GatewayConfig struct {
	Port int    `yaml:"port"`
	Bind string `yaml:"bind"`
}

type AgentConfig struct {
	Model           string `yaml:"model"`
	Provider        string `yaml:"provider"`
	APIKey          string `yaml:"apiKey"`
	AuthToken       string `yaml:"authToken"`  // OAuth setup-token (sk-ant-oat-...) for subscription auth
	MaxTokens       int    `yaml:"maxTokens"`
	System          string `yaml:"system"`
	FallbackModel   string `yaml:"fallbackModel"`    // Fallback model if primary fails
	FallbackProvider string `yaml:"fallbackProvider"` // Fallback provider (defaults to same as primary)
}

type ChannelsConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Discord  DiscordConfig  `yaml:"discord"`
}

type DiscordConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"token"`
}

type TelegramConfig struct {
	Enabled  bool   `yaml:"enabled"`
	BotToken string `yaml:"token"`
}

type HooksConfig struct {
	Enabled bool   `yaml:"enabled"`
	Token   string `yaml:"token"`
	Path    string `yaml:"path"`
}

func Default() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Port: 18789,
			Bind: "localhost",
		},
		Agent: AgentConfig{
			Model:     "claude-sonnet-4-20250514",
			Provider:  "anthropic",
			MaxTokens: 4096,
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
