package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/FeelPulse/feelpulse/internal/agent"
	"github.com/FeelPulse/feelpulse/internal/config"
	"github.com/FeelPulse/feelpulse/internal/gateway"
	"github.com/FeelPulse/feelpulse/internal/memory"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		cmdSetup()
	case "start":
		cmdGatewayStart()
	case "stop":
		cmdGatewayStop()
	case "restart":
		cmdGatewayRestart()
	case "status":
		cmdGatewayStatus()
	case "logs":
		cmdGatewayLogs()
	case "reset":
		cmdReset()
	case "version", "-v", "--version":
		cmdVersion()
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
  fp <command>

Commands:
  setup          Initial setup (creates config, starts gateway daemon)
  start          Start gateway daemon
  stop           Stop gateway daemon
  restart        Restart gateway daemon
  status         Check gateway status
  logs           View gateway logs (live, Ctrl+C to exit)
  reset          Clear all memory and sessions (requires confirmation)
  version        Print version
  help           Show this help`)
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".feelpulse")
}

func pidFile() string {
	return filepath.Join(configDir(), "gateway.pid")
}

func logFile() string {
	return filepath.Join(configDir(), "gateway.log")
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFile())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func cmdSetup() {
	fmt.Printf("🫀 FeelPulse v%s - Setup\n\n", version)

	// Check if already configured
	cfg, err := config.Load()
	if err == nil && (cfg.Agent.APIKey != "" || cfg.Agent.AuthToken != "") {
		fmt.Println("⚠️  Configuration already exists.")
		fmt.Print("Reconfigure? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(input)), "y") {
			fmt.Println("Setup cancelled.")
			os.Exit(0)
		}
		fmt.Println()
	} else {
		cfg = config.Default()
	}

	// Configure authentication
	reader := bufio.NewReader(os.Stdin)
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
		cfg.Agent.APIKey = ""
		cfg.Agent.Provider = "anthropic"

		fmt.Println("\n✅ Subscription auth configured!")
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
		cfg.Agent.AuthToken = ""
		cfg.Agent.Provider = "anthropic"

		fmt.Println("\n✅ API key configured!")

	default:
		fmt.Fprintln(os.Stderr, "❌ Invalid choice")
		os.Exit(1)
	}

	// Save config
	path, err := config.Save(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	// Initialize workspace
	workspacePath := cfg.Workspace.Path
	if workspacePath == "" {
		workspacePath = memory.DefaultWorkspacePath()
	}
	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: failed to create workspace: %v\n", err)
	}

	fmt.Printf("\n✅ Config saved: %s\n", path)
	fmt.Printf("📂 Workspace: %s\n", workspacePath)

	// Start gateway daemon
	fmt.Println("\n🚀 Starting gateway daemon...")
	if err := startDaemon(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait a bit and check if it started
	time.Sleep(500 * time.Millisecond)
	pid, err := readPID()
	if err != nil || !isProcessRunning(pid) {
		fmt.Fprintln(os.Stderr, "❌ Gateway failed to start. Check logs: fp logs")
		os.Exit(1)
	}

	fmt.Printf("✅ Gateway started (PID: %d)\n\n", pid)
	fmt.Printf("📡 Gateway: http://%s:%d\n", cfg.Gateway.Bind, cfg.Gateway.Port)
	if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.BotToken != "" {
		fmt.Println("📱 Telegram: enabled")
	}
	fmt.Println()
	fmt.Println("📝 View logs: fp logs")
	fmt.Println("🔍 Check status: fp status")
	fmt.Println()
	fmt.Println("🎉 Setup complete!")
}

func startDaemon(cfg *config.Config) error {
	// Get current executable path
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	// Prepare log file (do NOT defer close - daemon needs it)
	logF, err := os.OpenFile(logFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	// Start detached process
	cmd := exec.Command(exe, "_internal_gateway_start")
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}

	if err := cmd.Start(); err != nil {
		logF.Close()
		return err
	}

	// Don't close logF - daemon process inherits the file descriptor
	// and continues writing to it

	// Write PID file
	return os.WriteFile(pidFile(), []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644)
}

func cmdGatewayLogs() {
	logPath := logFile()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Println("📭 No logs yet.")
		fmt.Println("\nLog file: " + logPath)
		return
	}

	// Default: follow mode (tail -f)
	// Use tail -f to continuously output logs
	cmd := exec.Command("tail", "-f", logPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func cmdGatewayStatus() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("❌ Gateway is not running (no PID file)")
		return
	}

	if !isProcessRunning(pid) {
		fmt.Printf("❌ Gateway is not running (stale PID: %d)\n", pid)
		fmt.Println("\n💡 Remove stale PID file: rm " + pidFile())
		return
	}

	fmt.Printf("✅ Gateway is running (PID: %d)\n", pid)

	// Load config to show details
	if cfg, err := config.Load(); err == nil {
		fmt.Printf("\n📡 Gateway: http://%s:%d\n", cfg.Gateway.Bind, cfg.Gateway.Port)
		if cfg.Channels.Telegram.Enabled {
			fmt.Println("📱 Telegram: enabled")
		}
		fmt.Printf("📂 Workspace: %s\n", cfg.Workspace.Path)
	}

	fmt.Println("\n📝 View logs: fp logs")
}

// stopGateway stops the gateway daemon (returns error if not running or failed)
func stopGateway() error {
	pid, err := readPID()
	if err != nil {
		return fmt.Errorf("gateway is not running (no PID file)")
	}

	if !isProcessRunning(pid) {
		os.Remove(pidFile())
		return fmt.Errorf("gateway is not running (stale PID: %d)", pid)
	}

	// Send SIGTERM
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %v", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %v", err)
	}

	// Wait for shutdown (up to 5 seconds)
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isProcessRunning(pid) {
			os.Remove(pidFile())
			return nil
		}
	}

	// Force kill if still running
	if isProcessRunning(pid) {
		process.Kill()
		time.Sleep(500 * time.Millisecond)
	}

	os.Remove(pidFile())
	return nil
}

func cmdGatewayStop() {
	err := stopGateway()
	if err != nil {
		if strings.Contains(err.Error(), "not running") {
			fmt.Printf("❌ %v\n", err)
			if strings.Contains(err.Error(), "stale PID") {
				fmt.Println("✅ Cleaned up stale PID file")
			}
			return
		}
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	pid, _ := readPID()
	fmt.Printf("✅ Gateway stopped (was PID: %d)\n", pid)
}

func cmdGatewayStart() {
	// Check if already running
	pid, err := readPID()
	if err == nil && isProcessRunning(pid) {
		fmt.Printf("⚠️  Gateway is already running (PID: %d)\n", pid)
		fmt.Println("Use 'fp restart' to restart it.")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error loading config: %v\n", err)
		fmt.Println("Run 'fp setup' to create a config file.")
		os.Exit(1)
	}

	fmt.Println("🚀 Starting gateway daemon...")

	// Start daemon
	if err := startDaemon(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait a bit and check if it started
	time.Sleep(500 * time.Millisecond)
	pid, err = readPID()
	if err != nil || !isProcessRunning(pid) {
		fmt.Fprintln(os.Stderr, "❌ Gateway failed to start. Check logs: fp logs")
		os.Exit(1)
	}

	fmt.Printf("✅ Gateway started (PID: %d)\n", pid)
	fmt.Printf("\n📡 Gateway: http://%s:%d\n", cfg.Gateway.Bind, cfg.Gateway.Port)
	if cfg.Channels.Telegram.Enabled && cfg.Channels.Telegram.BotToken != "" {
		fmt.Println("📱 Telegram: enabled")
	}
	fmt.Println("\n📝 View logs: fp logs")
	fmt.Println("🔍 Check status: fp status")
}

func cmdGatewayRestart() {
	fmt.Println("🔄 Restarting gateway...")

	// Stop if running
	pid, err := readPID()
	if err == nil && isProcessRunning(pid) {
		cmdGatewayStop()
		time.Sleep(1 * time.Second)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error loading config: %v\n", err)
		fmt.Println("Run 'fp setup' to create a config file.")
		os.Exit(1)
	}

	// Start daemon
	if err := startDaemon(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to start daemon: %v\n", err)
		os.Exit(1)
	}

	time.Sleep(500 * time.Millisecond)
	pid, err = readPID()
	if err != nil || !isProcessRunning(pid) {
		fmt.Fprintln(os.Stderr, "❌ Gateway failed to start. Check logs: fp logs")
		os.Exit(1)
	}

	fmt.Printf("✅ Gateway restarted (PID: %d)\n", pid)
}

func cmdReset() {
	// Check if gateway is running
	wasRunning := false
	if pid, err := readPID(); err == nil && isProcessRunning(pid) {
		wasRunning = true
	}

	// Load config to get workspace path
	var workspacePath string
	if cfg, err := config.Load(); err == nil && cfg.Workspace.Path != "" {
		workspacePath = cfg.Workspace.Path
	} else {
		workspacePath = memory.DefaultWorkspacePath()
	}

	// Show what will be deleted
	fmt.Println("⚠️  *Reset Confirmation Required*")
	fmt.Println()
	if wasRunning {
		fmt.Println("Gateway is currently running and will be stopped.")
		fmt.Println()
	}
	fmt.Println("This will:")
	fmt.Println("  - Clear ALL session history (conversations, reminders, sub-agents, pins)")
	fmt.Println("  - Remove IDENTITY.md, MEMORY.md, and memory/ directory")
	fmt.Println("  - Delete database: ~/.feelpulse/sessions.db")
	fmt.Println()
	fmt.Println("User config files are preserved:")
	fmt.Println("  - AGENTS.md, SOUL.md, USER.md, TOOLS.md, HEARTBEAT.md")
	fmt.Println()
	fmt.Println("⚠️  *This cannot be undone.*")
	fmt.Println()
	fmt.Print("Type 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "yes" {
		fmt.Println("❌ Reset cancelled.")
		os.Exit(0)
	}

	// Stop gateway if running
	if wasRunning {
		fmt.Println()
		fmt.Println("🛑 Stopping gateway...")
		if err := stopGateway(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Failed to stop gateway cleanly: %v\n", err)
			fmt.Println("   Proceeding with reset anyway...")
		} else {
			fmt.Println("✅ Gateway stopped")
		}
	}

	// Delete database file
	home, _ := os.UserHomeDir()
	dbPath := fmt.Sprintf("%s/.feelpulse/sessions.db", home)

	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Failed to delete database: %v\n", err)
		} else {
			fmt.Println("✅ Database cleared")
		}
	}

	// Reset memory files
	mgr := memory.NewManager(workspacePath)
	if err := mgr.Reset(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Memory reset failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Memory cleared")
	fmt.Println()
	fmt.Println("🎉 Reset complete!")
	fmt.Println()
	fmt.Println("To start fresh, run:")
	fmt.Println("  fp start")
}

func cmdVersion() {
	fmt.Printf("FeelPulse v%s\n", version)
}

// Internal command called by daemon
func internalGatewayStart() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	gw := gateway.New(cfg)
	if err := gw.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Handle internal gateway start command
	if len(os.Args) >= 2 && os.Args[1] == "_internal_gateway_start" {
		internalGatewayStart()
		os.Exit(0)
	}
}
