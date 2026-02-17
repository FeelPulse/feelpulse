package heartbeat

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewService(t *testing.T) {
	cfg := &Config{
		Enabled:         true,
		IntervalMinutes: 30,
	}

	svc := New(cfg, "")

	if svc == nil {
		t.Fatal("Expected non-nil service")
	}

	if !svc.cfg.Enabled {
		t.Error("Expected service to be enabled")
	}

	if svc.cfg.IntervalMinutes != 30 {
		t.Errorf("Expected interval 30, got %d", svc.cfg.IntervalMinutes)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Expected disabled by default")
	}

	if cfg.IntervalMinutes != DefaultIntervalMinutes {
		t.Errorf("Expected default interval %d, got %d", DefaultIntervalMinutes, cfg.IntervalMinutes)
	}
}

func TestService_RegisterUser(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	svc.RegisterUser("telegram", 12345)

	users := svc.ListActiveUsers()
	if len(users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(users))
	}

	if users[0].Channel != "telegram" || users[0].UserID != 12345 {
		t.Errorf("Unexpected user: %+v", users[0])
	}
}

func TestService_UnregisterUser(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	svc.RegisterUser("telegram", 12345)
	svc.RegisterUser("telegram", 67890)
	svc.UnregisterUser("telegram", 12345)

	users := svc.ListActiveUsers()
	if len(users) != 1 {
		t.Fatalf("Expected 1 user after unregister, got %d", len(users))
	}

	if users[0].UserID != 67890 {
		t.Errorf("Wrong user remaining: %+v", users[0])
	}
}

func TestService_GetGreeting(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	tests := []struct {
		hour     int
		contains string
	}{
		{6, "morning"},
		{12, "afternoon"},
		{18, "evening"},
		{23, "evening"},
	}

	for _, tt := range tests {
		greeting := svc.getTimeGreeting(tt.hour)
		if greeting == "" {
			t.Errorf("Expected non-empty greeting for hour %d", tt.hour)
		}
	}
}

func TestService_LoadHeartbeatMD(t *testing.T) {
	// Create temp workspace with HEARTBEAT.md
	tmpDir, err := os.MkdirTemp("", "heartbeat-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `# Heartbeat Tasks

- Check weather daily
- Send morning motivation
`
	if err := os.WriteFile(filepath.Join(tmpDir, "HEARTBEAT.md"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write HEARTBEAT.md: %v", err)
	}

	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, tmpDir)

	tasks := svc.LoadHeartbeatTasks()
	if tasks == "" {
		t.Error("Expected to load HEARTBEAT.md content")
	}

	if tasks != content {
		t.Errorf("Expected content to match. Got: %s", tasks)
	}
}

func TestService_LoadHeartbeatMD_NoFile(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "/nonexistent/path")

	tasks := svc.LoadHeartbeatTasks()
	if tasks != "" {
		t.Errorf("Expected empty string for missing file, got: %s", tasks)
	}
}

func TestService_HeartbeatMessage(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	// Without HEARTBEAT.md, should just get time-based greeting
	msg := svc.BuildHeartbeatMessage()

	if msg == "" {
		t.Error("Expected non-empty heartbeat message")
	}
}

func TestService_StartStop(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 1} // 1 minute for fast test
	svc := New(cfg, "")

	// Track calls
	var mu sync.Mutex
	calls := 0
	svc.SetCallback(func(channel string, userID int64, message string) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	// Register a user
	svc.RegisterUser("telegram", 12345)

	// Start service
	svc.Start()

	// Give it a moment (but not enough for actual heartbeat)
	time.Sleep(100 * time.Millisecond)

	// Stop should work
	svc.Stop()

	// Service should be stopped
	if svc.IsRunning() {
		t.Error("Expected service to be stopped")
	}
}

func TestService_DisabledNoStart(t *testing.T) {
	cfg := &Config{Enabled: false, IntervalMinutes: 60}
	svc := New(cfg, "")

	svc.Start()

	if svc.IsRunning() {
		t.Error("Service should not start when disabled")
	}
}

func TestService_UniqueUsers(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	// Register same user twice
	svc.RegisterUser("telegram", 12345)
	svc.RegisterUser("telegram", 12345)

	users := svc.ListActiveUsers()
	if len(users) != 1 {
		t.Errorf("Expected 1 unique user, got %d", len(users))
	}
}

func TestService_MultipleChannels(t *testing.T) {
	cfg := &Config{Enabled: true, IntervalMinutes: 60}
	svc := New(cfg, "")

	// Register users from different channels
	svc.RegisterUser("telegram", 12345)
	svc.RegisterUser("discord", 12345) // Same ID, different channel

	users := svc.ListActiveUsers()
	if len(users) != 2 {
		t.Errorf("Expected 2 users from different channels, got %d", len(users))
	}
}
