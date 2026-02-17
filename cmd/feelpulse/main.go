package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/gateway"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart()
	case "status":
		cmdStatus()
	case "init":
		cmdInit()
	case "auth":
		cmdAuth()
	case "version":
		fmt.Printf("feelpulse v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`FeelPulse — Fast AI Assistant Platform

Usage:
  feelpulse <command>

Commands:
  init      Initialize configuration
  start     Start the gateway server
  status    Check gateway status
  auth      Configure authentication (API key or setup-token)
  version   Print version
  help      Show this help`)
}

func cmdInit() {
	cfg := config.Default()
	path, err := config.Save(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Config created: %s\n", path)
}

func cmdStart() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		fmt.Println("Run 'feelpulse init' to create a config file.")
		os.Exit(1)
	}

	fmt.Printf("🫀 FeelPulse v%s\n", version)
	fmt.Printf("📡 Starting gateway on %s:%d\n", cfg.Gateway.Bind, cfg.Gateway.Port)
	
	// Show configured channels
	if cfg.Channels.Telegram.Enabled {
		fmt.Println("📱 Telegram channel enabled")
	}
	if cfg.Agent.AuthToken != "" {
		fmt.Printf("🤖 Agent: %s/%s (subscription)\n", cfg.Agent.Provider, cfg.Agent.Model)
	} else if cfg.Agent.APIKey != "" {
		fmt.Printf("🤖 Agent: %s/%s (api-key)\n", cfg.Agent.Provider, cfg.Agent.Model)
	}

	gw := gateway.New(cfg)
	if err := gw.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdAuth() {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("🔐 FeelPulse Auth Setup")
	fmt.Println()
	fmt.Println("Choose authentication method:")
	fmt.Println("  1) API Key (pay-per-token)")
	fmt.Println("  2) Setup Token (use Claude subscription)")
	fmt.Print("\nChoice [1/2]: ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "2":
		fmt.Println()
		fmt.Println("📋 Steps:")
		fmt.Println("  1. Run: claude setup-token")
		fmt.Println("  2. Copy the token (starts with sk-ant-oat-...)")
		fmt.Println("  3. Paste it below")
		fmt.Print("\nPaste setup-token: ")

		token, _ := reader.ReadString('\n')
		token = strings.TrimSpace(token)

		if !agent.IsOAuthToken(token) {
			fmt.Fprintln(os.Stderr, "❌ Invalid setup-token (should start with sk-ant-oat)")
			os.Exit(1)
		}

		cfg.Agent.AuthToken = token
		cfg.Agent.APIKey = "" // clear API key
		cfg.Agent.Provider = "anthropic"

		path, err := config.Save(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Subscription auth configured! (%s)\n", path)
		fmt.Println("💡 No API fees — uses your Claude subscription quota.")

	case "1", "":
		fmt.Print("\nPaste API key (sk-ant-api-...): ")

		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)

		if key == "" {
			fmt.Fprintln(os.Stderr, "❌ No key provided")
			os.Exit(1)
		}

		cfg.Agent.APIKey = key
		cfg.Agent.AuthToken = "" // clear setup-token
		cfg.Agent.Provider = "anthropic"

		path, err := config.Save(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ API key configured! (%s)\n", path)

	default:
		fmt.Fprintln(os.Stderr, "❌ Invalid choice")
		os.Exit(1)
	}
}

func cmdStatus() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("❌ Not configured. Run 'feelpulse init'.")
		os.Exit(1)
	}

	fmt.Printf("🫀 FeelPulse v%s\n", version)
	fmt.Printf("📡 Gateway: http://%s:%d\n", cfg.Gateway.Bind, cfg.Gateway.Port)
	
	// Show configuration status
	fmt.Println("\n📋 Configuration:")
	if cfg.Agent.AuthToken != "" {
		fmt.Printf("   🤖 Agent: %s/%s (subscription auth)\n", cfg.Agent.Provider, cfg.Agent.Model)
	} else if cfg.Agent.APIKey != "" {
		fmt.Printf("   🤖 Agent: %s/%s (api-key)\n", cfg.Agent.Provider, cfg.Agent.Model)
	} else {
		fmt.Println("   🤖 Agent: Not configured (run 'feelpulse auth')")
	}
	
	if cfg.Channels.Telegram.Enabled {
		if cfg.Channels.Telegram.BotToken != "" {
			fmt.Println("   📱 Telegram: Configured")
		} else {
			fmt.Println("   📱 Telegram: Enabled but no token")
		}
	} else {
		fmt.Println("   📱 Telegram: Disabled")
	}
	
	fmt.Println("\n✅ Config loaded")
}
