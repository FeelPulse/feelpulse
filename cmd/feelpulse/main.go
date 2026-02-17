package main

import (
	"fmt"
	"os"

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
	if cfg.Agent.APIKey != "" {
		fmt.Printf("🤖 Agent: %s/%s\n", cfg.Agent.Provider, cfg.Agent.Model)
	}

	gw := gateway.New(cfg)
	if err := gw.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
	if cfg.Agent.APIKey != "" {
		fmt.Printf("   🤖 Agent: %s/%s\n", cfg.Agent.Provider, cfg.Agent.Model)
	} else {
		fmt.Println("   🤖 Agent: Not configured (set apiKey in config.yaml)")
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
