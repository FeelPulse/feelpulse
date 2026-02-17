package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FeelPulse/feelpulse/pkg/types"
)

func TestSQLiteStore_CreateAndClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestSQLiteStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Create a session with messages
	key := "telegram:12345"
	messages := []types.Message{
		{
			ID:        "1",
			Text:      "Hello",
			From:      "user",
			Channel:   "telegram",
			IsBot:     false,
			Timestamp: time.Now(),
		},
		{
			ID:        "2",
			Text:      "Hi there!",
			From:      "bot",
			Channel:   "telegram",
			IsBot:     true,
			Timestamp: time.Now(),
		},
	}
	model := "claude-sonnet-4-20250514"

	// Save session
	err = store.Save(key, messages, model)
	if err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Load session
	loadedMessages, loadedModel, err := store.Load(key)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	if loadedModel != model {
		t.Errorf("model mismatch: got %s, want %s", loadedModel, model)
	}

	if len(loadedMessages) != len(messages) {
		t.Fatalf("message count mismatch: got %d, want %d", len(loadedMessages), len(messages))
	}

	for i, msg := range loadedMessages {
		if msg.Text != messages[i].Text {
			t.Errorf("message %d text mismatch: got %s, want %s", i, msg.Text, messages[i].Text)
		}
		if msg.From != messages[i].From {
			t.Errorf("message %d from mismatch: got %s, want %s", i, msg.From, messages[i].From)
		}
		if msg.IsBot != messages[i].IsBot {
			t.Errorf("message %d isBot mismatch: got %v, want %v", i, msg.IsBot, messages[i].IsBot)
		}
	}
}

func TestSQLiteStore_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Load non-existent session
	messages, model, err := store.Load("nonexistent:key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if messages != nil {
		t.Error("expected nil messages for non-existent session")
	}
	if model != "" {
		t.Error("expected empty model for non-existent session")
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	key := "telegram:12345"
	messages := []types.Message{
		{ID: "1", Text: "Hello", From: "user", Channel: "telegram"},
	}

	// Save and verify
	err = store.Save(key, messages, "")
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, _, err := store.Load(key)
	if err != nil || loaded == nil {
		t.Fatal("session should exist after save")
	}

	// Delete
	err = store.Delete(key)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Verify deleted
	loaded, _, err = store.Load(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded != nil {
		t.Error("session should be nil after delete")
	}
}

func TestSQLiteStore_ListKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Save multiple sessions
	keys := []string{"telegram:1", "telegram:2", "discord:1"}
	for _, key := range keys {
		err = store.Save(key, []types.Message{{ID: "1", Text: "test"}}, "")
		if err != nil {
			t.Fatalf("failed to save: %v", err)
		}
	}

	// List all keys
	allKeys, err := store.ListKeys()
	if err != nil {
		t.Fatalf("failed to list keys: %v", err)
	}

	if len(allKeys) != len(keys) {
		t.Errorf("key count mismatch: got %d, want %d", len(allKeys), len(keys))
	}
}

func TestSQLiteStore_UpdateExisting(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	key := "telegram:12345"

	// Save initial
	err = store.Save(key, []types.Message{{ID: "1", Text: "first"}}, "model1")
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Update with new messages
	err = store.Save(key, []types.Message{
		{ID: "1", Text: "first"},
		{ID: "2", Text: "second"},
	}, "model2")
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	// Verify update
	messages, model, err := store.Load(key)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
	if model != "model2" {
		t.Errorf("expected model2, got %s", model)
	}
}

func TestSQLiteStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store, save data, close
	store1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	key := "telegram:persist"
	messages := []types.Message{{ID: "1", Text: "persistent"}}
	err = store1.Save(key, messages, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}
	store1.Close()

	// Open new store, verify data persisted
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create second store: %v", err)
	}
	defer store2.Close()

	loaded, model, err := store2.Load(key)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if len(loaded) != 1 {
		t.Errorf("expected 1 message, got %d", len(loaded))
	}
	if loaded[0].Text != "persistent" {
		t.Errorf("expected 'persistent', got '%s'", loaded[0].Text)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("expected 'claude-sonnet-4-20250514', got '%s'", model)
	}
}

// === Reminder Persistence Tests ===

func TestSQLiteStore_ReminderTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	err = store.EnsureRemindersTable()
	if err != nil {
		t.Fatalf("failed to create reminders table: %v", err)
	}

	// Should be able to call again without error (idempotent)
	err = store.EnsureRemindersTable()
	if err != nil {
		t.Fatalf("second call to EnsureRemindersTable failed: %v", err)
	}
}

func TestSQLiteStore_SaveAndLoadReminder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	err = store.EnsureRemindersTable()
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	reminder := &ReminderData{
		ID:      "test-123",
		Channel: "telegram",
		UserID:  "user456",
		Message: "Remember this",
		FireAt:  now.Add(1 * time.Hour),
		Created: now,
	}

	// Save
	err = store.SaveReminder(reminder)
	if err != nil {
		t.Fatalf("failed to save reminder: %v", err)
	}

	// Load
	reminders, err := store.LoadReminders()
	if err != nil {
		t.Fatalf("failed to load reminders: %v", err)
	}

	if len(reminders) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(reminders))
	}

	r := reminders[0]
	if r.ID != reminder.ID {
		t.Errorf("ID mismatch: got %s, want %s", r.ID, reminder.ID)
	}
	if r.Channel != reminder.Channel {
		t.Errorf("Channel mismatch: got %s, want %s", r.Channel, reminder.Channel)
	}
	if r.UserID != reminder.UserID {
		t.Errorf("UserID mismatch: got %s, want %s", r.UserID, reminder.UserID)
	}
	if r.Message != reminder.Message {
		t.Errorf("Message mismatch: got %s, want %s", r.Message, reminder.Message)
	}
}

func TestSQLiteStore_DeleteReminder(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	err = store.EnsureRemindersTable()
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	reminder := &ReminderData{
		ID:      "delete-me",
		Channel: "telegram",
		UserID:  "user789",
		Message: "To be deleted",
		FireAt:  now.Add(1 * time.Hour),
		Created: now,
	}

	// Save and verify
	_ = store.SaveReminder(reminder)
	reminders, _ := store.LoadReminders()
	if len(reminders) != 1 {
		t.Fatal("reminder should exist after save")
	}

	// Delete
	err = store.DeleteReminder(reminder.ID)
	if err != nil {
		t.Fatalf("failed to delete reminder: %v", err)
	}

	// Verify deleted
	reminders, _ = store.LoadReminders()
	if len(reminders) != 0 {
		t.Error("reminder should be deleted")
	}
}

func TestSQLiteStore_ReminderPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create store, save reminder, close
	store1, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	_ = store1.EnsureRemindersTable()
	reminder := &ReminderData{
		ID:      "persist-reminder",
		Channel: "telegram",
		UserID:  "user",
		Message: "Persistent reminder",
		FireAt:  time.Now().Add(1 * time.Hour),
		Created: time.Now(),
	}
	_ = store1.SaveReminder(reminder)
	store1.Close()

	// Open new store, verify persistence
	store2, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create second store: %v", err)
	}
	defer store2.Close()

	_ = store2.EnsureRemindersTable()
	reminders, err := store2.LoadReminders()
	if err != nil {
		t.Fatalf("failed to load reminders: %v", err)
	}

	if len(reminders) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(reminders))
	}
	if reminders[0].Message != "Persistent reminder" {
		t.Errorf("expected 'Persistent reminder', got '%s'", reminders[0].Message)
	}
}

func TestSQLiteStore_CleanExpiredReminders(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_ = store.EnsureRemindersTable()

	now := time.Now()

	// Add one expired and one future reminder
	expired := &ReminderData{
		ID:      "expired",
		Channel: "telegram",
		UserID:  "user",
		Message: "Expired",
		FireAt:  now.Add(-1 * time.Hour), // Past
		Created: now.Add(-2 * time.Hour),
	}
	future := &ReminderData{
		ID:      "future",
		Channel: "telegram",
		UserID:  "user",
		Message: "Future",
		FireAt:  now.Add(1 * time.Hour), // Future
		Created: now,
	}

	_ = store.SaveReminder(expired)
	_ = store.SaveReminder(future)

	// Clean expired
	count, err := store.CleanExpiredReminders()
	if err != nil {
		t.Fatalf("failed to clean: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 deleted, got %d", count)
	}

	// Verify only future remains
	reminders, _ := store.LoadReminders()
	if len(reminders) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(reminders))
	}
	if reminders[0].ID != "future" {
		t.Errorf("wrong reminder remaining: %s", reminders[0].ID)
	}
}
