package watcher

import (
	"os"
	"sync"
	"time"
)

const (
	// DefaultPollInterval is the default interval for checking config changes
	DefaultPollInterval = 5 * time.Second
)

// ConfigWatcher watches a config file for changes using polling
type ConfigWatcher struct {
	path         string
	pollInterval time.Duration
	callback     func()
	lastModTime  time.Time
	lastSize     int64
	stop         chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
}

// NewConfigWatcher creates a new config file watcher
func NewConfigWatcher(path string, pollInterval time.Duration) *ConfigWatcher {
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	return &ConfigWatcher{
		path:         path,
		pollInterval: pollInterval,
		stop:         make(chan struct{}),
	}
}

// SetCallback sets the function to call when config changes
func (w *ConfigWatcher) SetCallback(fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callback = fn
}

// Start begins watching the config file
func (w *ConfigWatcher) Start() {
	// Get initial file state
	w.updateFileState()

	w.wg.Add(1)
	go w.watch()
}

// Stop stops watching the config file
func (w *ConfigWatcher) Stop() {
	close(w.stop)
	w.wg.Wait()
}

// updateFileState updates the cached file modification time and size
func (w *ConfigWatcher) updateFileState() {
	info, err := os.Stat(w.path)
	if err != nil {
		// File doesn't exist yet - reset state
		w.lastModTime = time.Time{}
		w.lastSize = 0
		return
	}
	w.lastModTime = info.ModTime()
	w.lastSize = info.Size()
}

// hasChanged checks if the file has changed since last check
func (w *ConfigWatcher) hasChanged() bool {
	info, err := os.Stat(w.path)
	if err != nil {
		// File doesn't exist
		if w.lastModTime.IsZero() {
			// File didn't exist before either
			return false
		}
		// File was deleted - that's a change
		return true
	}

	// Check if file appeared (was previously missing)
	if w.lastModTime.IsZero() {
		return true
	}

	// Check modification time or size change
	return !info.ModTime().Equal(w.lastModTime) || info.Size() != w.lastSize
}

// watch is the main polling loop
func (w *ConfigWatcher) watch() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			if w.hasChanged() {
				w.updateFileState()

				w.mu.Lock()
				callback := w.callback
				w.mu.Unlock()

				if callback != nil {
					callback()
				}
			}
		}
	}
}
