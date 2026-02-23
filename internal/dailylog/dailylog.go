package dailylog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

// Writer handles writing messages to daily log files
type Writer struct {
	basePath string
	enabled  bool
}

// NewWriter creates a new daily log writer
func NewWriter(workspacePath string, enabled bool) *Writer {
	return &Writer{
		basePath: filepath.Join(workspacePath, "memory"),
		enabled:  enabled,
	}
}

// Log writes a message to today's log file
func (w *Writer) Log(msg types.Message) error {
	if !w.enabled {
		return nil
	}

	// Ensure memory directory exists
	if err := os.MkdirAll(w.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create memory directory: %w", err)
	}

	// Get today's log file path
	date := time.Now().Format("2006-01-02")
	logPath := filepath.Join(w.basePath, date+".md")

	// Format log entry
	entry := formatLogEntry(msg)

	// Append to file
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open daily log: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("failed to write to daily log: %w", err)
	}

	return nil
}

// formatLogEntry formats a message as a markdown log entry
func formatLogEntry(msg types.Message) string {
	timestamp := msg.Timestamp.Format("15:04:05")
	role := "User"
	if msg.IsBot {
		role = "Bot"
	}

	// Format:
	// ## 15:04:05 [User]
	// Message text
	//
	return fmt.Sprintf("## %s [%s]\n%s\n\n", timestamp, role, msg.Text)
}

// Enable enables daily logging
func (w *Writer) Enable() {
	w.enabled = true
}

// Disable disables daily logging
func (w *Writer) Disable() {
	w.enabled = false
}

// IsEnabled returns whether daily logging is enabled
func (w *Writer) IsEnabled() bool {
	return w.enabled
}
