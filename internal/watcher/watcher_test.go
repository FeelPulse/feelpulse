package watcher

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewConfigWatcher(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config file
	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	w := NewConfigWatcher(configPath, 100*time.Millisecond)
	if w == nil {
		t.Fatal("NewConfigWatcher returned nil")
	}
	if w.path != configPath {
		t.Errorf("path = %q, want %q", w.path, configPath)
	}
}

func TestConfigWatcher_DetectsChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config file
	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	var callbackCount int32
	callback := func() {
		atomic.AddInt32(&callbackCount, 1)
	}

	w := NewConfigWatcher(configPath, 50*time.Millisecond)
	w.SetCallback(callback)

	// Start watching
	w.Start()
	defer w.Stop()

	// Wait a bit for initial state
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(configPath, []byte("test: 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for detection
	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt32(&callbackCount)
	if count == 0 {
		t.Error("Callback should have been called after file change")
	}
}

func TestConfigWatcher_NoCallbackIfUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create initial config file
	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	var callbackCount int32
	callback := func() {
		atomic.AddInt32(&callbackCount, 1)
	}

	w := NewConfigWatcher(configPath, 50*time.Millisecond)
	w.SetCallback(callback)

	// Start watching
	w.Start()
	defer w.Stop()

	// Wait for several poll cycles without changes
	time.Sleep(200 * time.Millisecond)

	count := atomic.LoadInt32(&callbackCount)
	if count != 0 {
		t.Errorf("Callback should not be called without changes, got %d calls", count)
	}
}

func TestConfigWatcher_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	var callbackCount int32
	callback := func() {
		atomic.AddInt32(&callbackCount, 1)
	}

	w := NewConfigWatcher(configPath, 50*time.Millisecond)
	w.SetCallback(callback)

	w.Start()
	time.Sleep(100 * time.Millisecond)
	w.Stop()

	// Modify file after stopping
	if err := os.WriteFile(configPath, []byte("test: 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait and verify no callbacks
	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt32(&callbackCount)
	if count != 0 {
		t.Errorf("Callback should not be called after Stop, got %d calls", count)
	}
}

func TestConfigWatcher_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.yaml")

	var callbackCount int32
	callback := func() {
		atomic.AddInt32(&callbackCount, 1)
	}

	w := NewConfigWatcher(configPath, 50*time.Millisecond)
	w.SetCallback(callback)

	// Should not panic with missing file
	w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create the file
	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for detection
	time.Sleep(150 * time.Millisecond)

	count := atomic.LoadInt32(&callbackCount)
	if count == 0 {
		t.Error("Callback should be called when file appears")
	}
}

func TestConfigWatcher_MultipleChanges(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("test: 1"), 0644); err != nil {
		t.Fatal(err)
	}

	var callbackCount int32
	callback := func() {
		atomic.AddInt32(&callbackCount, 1)
	}

	w := NewConfigWatcher(configPath, 50*time.Millisecond)
	w.SetCallback(callback)

	w.Start()
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Make multiple changes
	for i := 2; i <= 4; i++ {
		content := []byte("test: " + string(rune('0'+i)))
		if err := os.WriteFile(configPath, content, 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	count := atomic.LoadInt32(&callbackCount)
	if count < 3 {
		t.Errorf("Callback should be called multiple times, got %d calls", count)
	}
}

func TestDefaultPollInterval(t *testing.T) {
	if DefaultPollInterval != 5*time.Second {
		t.Errorf("DefaultPollInterval = %v, want 5s", DefaultPollInterval)
	}
}
