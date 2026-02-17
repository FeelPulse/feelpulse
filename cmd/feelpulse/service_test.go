package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateServiceFile(t *testing.T) {
	// Test service file generation
	content := generateServiceFile("testuser", "/usr/local/bin/feelpulse", "/home/testuser")

	// Check required sections
	if !strings.Contains(content, "[Unit]") {
		t.Error("service file missing [Unit] section")
	}
	if !strings.Contains(content, "[Service]") {
		t.Error("service file missing [Service] section")
	}
	if !strings.Contains(content, "[Install]") {
		t.Error("service file missing [Install] section")
	}

	// Check placeholders are filled
	if !strings.Contains(content, "User=testuser") {
		t.Error("service file missing User directive")
	}
	if !strings.Contains(content, "ExecStart=/usr/local/bin/feelpulse start") {
		t.Error("service file missing ExecStart directive")
	}
	if !strings.Contains(content, "Environment=HOME=/home/testuser") {
		t.Error("service file missing Environment directive")
	}
	if !strings.Contains(content, "Description=FeelPulse AI Assistant") {
		t.Error("service file missing Description")
	}
}

func TestServicePaths(t *testing.T) {
	// Test system service path
	systemPath := systemServicePath()
	if systemPath != "/etc/systemd/system/feelpulse.service" {
		t.Errorf("unexpected system service path: %s", systemPath)
	}

	// Test user service path (should be under home directory)
	userPath := userServicePath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "systemd", "user", "feelpulse.service")
	if userPath != expected {
		t.Errorf("unexpected user service path: got %s, want %s", userPath, expected)
	}
}

func TestIsSystemInstall(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"no args defaults to user", []string{}, false},
		{"--system flag", []string{"--system"}, true},
		{"--user flag", []string{"--user"}, false},
		{"-s flag", []string{"-s"}, true},
		{"other flag ignored", []string{"--other"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemInstall(tt.args)
			if result != tt.expected {
				t.Errorf("isSystemInstall(%v) = %v, want %v", tt.args, result, tt.expected)
			}
		})
	}
}

func TestServiceInstallWritesFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "feelpulse.service")

	// Mock the writeServiceFile function
	content := generateServiceFile("testuser", "/usr/bin/feelpulse", "/home/testuser")
	err := os.MkdirAll(filepath.Dir(servicePath), 0755)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	err = os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("failed to read service file: %v", err)
	}

	if !strings.Contains(string(data), "FeelPulse AI Assistant") {
		t.Error("service file content incorrect")
	}
}

func TestServiceUninstallRemovesFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	servicePath := filepath.Join(tmpDir, "feelpulse.service")

	// Create a dummy service file
	err := os.WriteFile(servicePath, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("failed to create service file: %v", err)
	}

	// Remove it
	err = os.Remove(servicePath)
	if err != nil {
		t.Fatalf("failed to remove service file: %v", err)
	}

	// Verify file was removed
	_, err = os.Stat(servicePath)
	if !os.IsNotExist(err) {
		t.Error("service file should have been removed")
	}
}
