package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

const serviceTemplate = `[Unit]
Description=FeelPulse AI Assistant
After=network.target

[Service]
Type=simple
User=%s
ExecStart=%s start
Restart=on-failure
RestartSec=5s
Environment=HOME=%s

[Install]
WantedBy=default.target
`

// generateServiceFile creates the systemd service file content
func generateServiceFile(username, execPath, homeDir string) string {
	return fmt.Sprintf(serviceTemplate, username, execPath, homeDir)
}

// systemServicePath returns the path for system-wide service
func systemServicePath() string {
	return "/etc/systemd/system/feelpulse.service"
}

// userServicePath returns the path for user service
func userServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "feelpulse.service")
}

// isSystemInstall checks if --system or -s flag is present
func isSystemInstall(args []string) bool {
	for _, arg := range args {
		if arg == "--system" || arg == "-s" {
			return true
		}
	}
	return false
}

// getServicePath returns the appropriate service path based on flags
func getServicePath(system bool) string {
	if system {
		return systemServicePath()
	}
	return userServicePath()
}

// getExecutablePath returns the path to the feelpulse binary
func getExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return filepath.Abs(exe)
}

// cmdService handles the service subcommand
func cmdService() {
	if len(os.Args) < 3 {
		printServiceUsage()
		os.Exit(1)
	}

	args := []string{}
	if len(os.Args) > 3 {
		args = os.Args[3:]
	}

	switch os.Args[2] {
	case "install":
		cmdServiceInstall(args)
	case "uninstall":
		cmdServiceUninstall(args)
	case "enable":
		cmdServiceEnable(args)
	case "disable":
		cmdServiceDisable(args)
	case "status":
		cmdServiceStatus(args)
	case "help", "-h", "--help":
		printServiceUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown service command: %s\n", os.Args[2])
		printServiceUsage()
		os.Exit(1)
	}
}

func printServiceUsage() {
	fmt.Println(`Usage: feelpulse service <command> [flags]

Commands:
  install     Install systemd service file
  uninstall   Remove systemd service file
  enable      Enable service to start on boot
  disable     Disable service autostart
  status      Show service status

Flags:
  --system, -s   System-wide service (requires root)
                 Default: user service (~/.config/systemd/user/)`)
}

func cmdServiceInstall(args []string) {
	system := isSystemInstall(args)
	servicePath := getServicePath(system)

	// Get current user info
	currentUser, err := user.Current()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to get current user: %v\n", err)
		os.Exit(1)
	}

	// Get executable path
	execPath, err := getExecutablePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// For user service, use current user; for system service, also use current user
	username := currentUser.Username
	homeDir := currentUser.HomeDir

	// Generate service file content
	content := generateServiceFile(username, execPath, homeDir)

	// Create directory if needed (for user service)
	dir := filepath.Dir(servicePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Write service file
	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		if os.IsPermission(err) && system {
			fmt.Fprintln(os.Stderr, "Error: permission denied. Run with sudo for system service.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: failed to write service file: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("✅ Service file installed: %s\n", servicePath)

	// Reload systemd daemon
	if system {
		fmt.Println("Reloading systemd daemon...")
		exec.Command("systemctl", "daemon-reload").Run()
	} else {
		fmt.Println("Reloading user systemd daemon...")
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	if system {
		fmt.Println("\nTo enable and start:")
		fmt.Println("  sudo systemctl enable feelpulse")
		fmt.Println("  sudo systemctl start feelpulse")
	} else {
		fmt.Println("\nTo enable and start:")
		fmt.Println("  systemctl --user enable feelpulse")
		fmt.Println("  systemctl --user start feelpulse")
	}
}

func cmdServiceUninstall(args []string) {
	system := isSystemInstall(args)
	servicePath := getServicePath(system)

	// Check if file exists
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		fmt.Println("⚠️ Service file not found (not installed?)")
		return
	}

	// Stop and disable service first
	if system {
		exec.Command("systemctl", "stop", "feelpulse").Run()
		exec.Command("systemctl", "disable", "feelpulse").Run()
	} else {
		exec.Command("systemctl", "--user", "stop", "feelpulse").Run()
		exec.Command("systemctl", "--user", "disable", "feelpulse").Run()
	}

	// Remove service file
	if err := os.Remove(servicePath); err != nil {
		if os.IsPermission(err) && system {
			fmt.Fprintln(os.Stderr, "Error: permission denied. Run with sudo for system service.")
		} else {
			fmt.Fprintf(os.Stderr, "Error: failed to remove service file: %v\n", err)
		}
		os.Exit(1)
	}

	// Reload daemon
	if system {
		exec.Command("systemctl", "daemon-reload").Run()
	} else {
		exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	fmt.Println("✅ Service uninstalled")
}

func cmdServiceEnable(args []string) {
	system := isSystemInstall(args)

	var cmd *exec.Cmd
	if system {
		cmd = exec.Command("systemctl", "enable", "feelpulse")
	} else {
		cmd = exec.Command("systemctl", "--user", "enable", "feelpulse")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if system {
			fmt.Fprintln(os.Stderr, "Tip: Run with sudo for system service")
		}
		os.Exit(1)
	}
}

func cmdServiceDisable(args []string) {
	system := isSystemInstall(args)

	var cmd *exec.Cmd
	if system {
		cmd = exec.Command("systemctl", "disable", "feelpulse")
	} else {
		cmd = exec.Command("systemctl", "--user", "disable", "feelpulse")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		if system {
			fmt.Fprintln(os.Stderr, "Tip: Run with sudo for system service")
		}
		os.Exit(1)
	}
}

func cmdServiceStatus(args []string) {
	system := isSystemInstall(args)

	var cmd *exec.Cmd
	if system {
		cmd = exec.Command("systemctl", "status", "feelpulse")
	} else {
		cmd = exec.Command("systemctl", "--user", "status", "feelpulse")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Don't check error - systemctl status returns non-zero if service is stopped
	cmd.Run()
}

// HasServiceArg checks if the "service" subcommand is being used
func HasServiceArg() bool {
	return len(os.Args) >= 2 && strings.ToLower(os.Args[1]) == "service"
}
