package session

import (
	"testing"
)

func TestSessionSetModel(t *testing.T) {
	store := NewStore()
	sess := store.GetOrCreate("telegram", "user123")

	// Default model should be empty (use config default)
	if sess.Model != "" {
		t.Errorf("Expected empty default model, got %s", sess.Model)
	}

	// Set a custom model
	sess.SetModel("gpt-4o")
	if sess.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got %s", sess.Model)
	}

	// Retrieve session and verify model persists
	sess2 := store.GetOrCreate("telegram", "user123")
	if sess2.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o' on second get, got %s", sess2.Model)
	}
}

func TestSessionClearResetsModel(t *testing.T) {
	store := NewStore()
	sess := store.GetOrCreate("telegram", "user123")

	sess.SetModel("claude-opus-4")
	sess.Clear()

	if sess.Model != "" {
		t.Errorf("Expected empty model after clear, got %s", sess.Model)
	}
}

func TestValidateModel(t *testing.T) {
	tests := []struct {
		model   string
		valid   bool
	}{
		{"claude-sonnet-4-20250514", true},
		{"claude-opus-4-20250514", true},
		{"claude-3-5-sonnet-20241022", true},
		{"claude-3-opus-20240229", true},
		{"invalid-model", false},
		{"", false},
		{"gpt-4o", false}, // Not supported (Anthropic only)
	}

	for _, tt := range tests {
		got := ValidateModel(tt.model)
		if got != tt.valid {
			t.Errorf("ValidateModel(%q) = %v, want %v", tt.model, got, tt.valid)
		}
	}
}

func TestSupportedModels(t *testing.T) {
	models := SupportedModels()
	if len(models) == 0 {
		t.Error("Expected some supported models")
	}

	// Check some expected models
	found := false
	for _, m := range models {
		if m == "claude-sonnet-4-20250514" || m == "gpt-4o" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find common models in supported list")
	}
}
