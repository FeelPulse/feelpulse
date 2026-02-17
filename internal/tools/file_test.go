package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFilePath(t *testing.T) {
	// Create temp workspace
	workspace, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workspace)

	tests := []struct {
		name          string
		workspacePath string
		requestedPath string
		expectError   bool
		errorContains string
	}{
		{
			name:          "valid simple path",
			workspacePath: workspace,
			requestedPath: "file.txt",
			expectError:   false,
		},
		{
			name:          "valid nested path",
			workspacePath: workspace,
			requestedPath: "subdir/file.txt",
			expectError:   false,
		},
		{
			name:          "path traversal with ..",
			workspacePath: workspace,
			requestedPath: "../outside.txt",
			expectError:   true,
			errorContains: "path traversal denied",
		},
		{
			name:          "path traversal with nested ..",
			workspacePath: workspace,
			requestedPath: "subdir/../../outside.txt",
			expectError:   true,
			errorContains: "path traversal denied",
		},
		{
			name:          "absolute path outside workspace",
			workspacePath: workspace,
			requestedPath: "/etc/passwd",
			expectError:   true,
			errorContains: "path traversal denied",
		},
		{
			name:          "hidden path traversal",
			workspacePath: workspace,
			requestedPath: "subdir/../../../etc/passwd",
			expectError:   true,
			errorContains: "path traversal denied",
		},
		{
			name:          "empty workspace path",
			workspacePath: "",
			requestedPath: "file.txt",
			expectError:   true,
			errorContains: "workspace path not configured",
		},
		{
			name:          "dot path is valid (workspace root)",
			workspacePath: workspace,
			requestedPath: ".",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateFilePath(tt.workspacePath, tt.requestedPath)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestFileReadTool(t *testing.T) {
	// Create temp workspace
	workspace, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workspace)

	// Create test file
	testContent := "Hello, World!"
	testFile := filepath.Join(workspace, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a subdirectory
	subdir := filepath.Join(workspace, "subdir")
	os.MkdirAll(subdir, 0755)

	cfg := &FileConfig{
		Enabled:       true,
		WorkspacePath: workspace,
	}

	tool := fileReadTool(cfg)
	ctx := context.Background()

	// Test reading existing file
	t.Run("read existing file", func(t *testing.T) {
		result, err := tool.Handler(ctx, map[string]any{"path": "test.txt"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if result != testContent {
			t.Errorf("Expected %q, got %q", testContent, result)
		}
	})

	// Test reading non-existent file
	t.Run("read non-existent file", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "nonexistent.txt"})
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	// Test reading directory
	t.Run("read directory", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "subdir"})
		if err == nil {
			t.Error("Expected error when reading directory")
		}
	})

	// Test path traversal
	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "../../../etc/passwd"})
		if err == nil {
			t.Error("Expected error for path traversal")
		}
	})

	// Test missing path parameter
	t.Run("missing path parameter", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{})
		if err == nil {
			t.Error("Expected error for missing path")
		}
	})
}

func TestFileWriteTool(t *testing.T) {
	// Create temp workspace
	workspace, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workspace)

	cfg := &FileConfig{
		Enabled:       true,
		WorkspacePath: workspace,
	}

	tool := fileWriteTool(cfg)
	ctx := context.Background()

	// Test writing new file
	t.Run("write new file", func(t *testing.T) {
		content := "Test content"
		_, err := tool.Handler(ctx, map[string]any{
			"path":    "newfile.txt",
			"content": content,
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(filepath.Join(workspace, "newfile.txt"))
		if err != nil {
			t.Errorf("Failed to read created file: %v", err)
		}
		if string(data) != content {
			t.Errorf("Expected %q, got %q", content, string(data))
		}
	})

	// Test creating nested directories
	t.Run("write with nested directories", func(t *testing.T) {
		content := "Nested content"
		_, err := tool.Handler(ctx, map[string]any{
			"path":    "deep/nested/dir/file.txt",
			"content": content,
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Verify file was created
		data, err := os.ReadFile(filepath.Join(workspace, "deep/nested/dir/file.txt"))
		if err != nil {
			t.Errorf("Failed to read created file: %v", err)
		}
		if string(data) != content {
			t.Errorf("Expected %q, got %q", content, string(data))
		}
	})

	// Test path traversal
	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{
			"path":    "../outside.txt",
			"content": "malicious",
		})
		if err == nil {
			t.Error("Expected error for path traversal")
		}
	})

	// Test overwriting file
	t.Run("overwrite existing file", func(t *testing.T) {
		content1 := "Original content"
		content2 := "New content"

		tool.Handler(ctx, map[string]any{"path": "overwrite.txt", "content": content1})
		tool.Handler(ctx, map[string]any{"path": "overwrite.txt", "content": content2})

		data, _ := os.ReadFile(filepath.Join(workspace, "overwrite.txt"))
		if string(data) != content2 {
			t.Errorf("Expected %q, got %q", content2, string(data))
		}
	})
}

func TestFileListTool(t *testing.T) {
	// Create temp workspace
	workspace, err := os.MkdirTemp("", "workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(workspace)

	// Create test structure
	os.WriteFile(filepath.Join(workspace, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(workspace, "file2.txt"), []byte("longer content for file 2"), 0644)
	os.MkdirAll(filepath.Join(workspace, "subdir"), 0755)
	os.WriteFile(filepath.Join(workspace, "subdir/nested.txt"), []byte("nested"), 0644)

	cfg := &FileConfig{
		Enabled:       true,
		WorkspacePath: workspace,
	}

	tool := fileListTool(cfg)
	ctx := context.Background()

	// Test listing workspace root
	t.Run("list root", func(t *testing.T) {
		result, err := tool.Handler(ctx, map[string]any{"path": "."})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !containsString(result, "file1.txt") || !containsString(result, "file2.txt") || !containsString(result, "subdir") {
			t.Errorf("Expected listing to contain files and directories, got: %s", result)
		}
	})

	// Test listing subdirectory
	t.Run("list subdirectory", func(t *testing.T) {
		result, err := tool.Handler(ctx, map[string]any{"path": "subdir"})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !containsString(result, "nested.txt") {
			t.Errorf("Expected listing to contain nested.txt, got: %s", result)
		}
	})

	// Test listing non-existent directory
	t.Run("list non-existent", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "nonexistent"})
		if err == nil {
			t.Error("Expected error for non-existent directory")
		}
	})

	// Test path traversal
	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "../"})
		if err == nil {
			t.Error("Expected error for path traversal")
		}
	})

	// Test listing file (should fail)
	t.Run("list file instead of directory", func(t *testing.T) {
		_, err := tool.Handler(ctx, map[string]any{"path": "file1.txt"})
		if err == nil {
			t.Error("Expected error when listing a file")
		}
	})
}

func TestRegisterFileTools(t *testing.T) {
	registry := NewRegistry()

	// Test with nil config
	t.Run("nil config", func(t *testing.T) {
		RegisterFileTools(registry, nil)
		if registry.Get("file_read") != nil {
			t.Error("Expected no tools registered with nil config")
		}
	})

	// Test with disabled config
	t.Run("disabled config", func(t *testing.T) {
		registry = NewRegistry()
		RegisterFileTools(registry, &FileConfig{Enabled: false})
		if registry.Get("file_read") != nil {
			t.Error("Expected no tools registered with disabled config")
		}
	})

	// Test with enabled config
	t.Run("enabled config", func(t *testing.T) {
		workspace, _ := os.MkdirTemp("", "workspace-*")
		defer os.RemoveAll(workspace)

		registry = NewRegistry()
		RegisterFileTools(registry, &FileConfig{
			Enabled:       true,
			WorkspacePath: workspace,
		})

		if registry.Get("file_read") == nil {
			t.Error("Expected file_read to be registered")
		}
		if registry.Get("file_write") == nil {
			t.Error("Expected file_write to be registered")
		}
		if registry.Get("file_list") == nil {
			t.Error("Expected file_list to be registered")
		}
	})
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		result := formatFileSize(tt.bytes)
		if result != tt.expected {
			t.Errorf("formatFileSize(%d) = %q, expected %q", tt.bytes, result, tt.expected)
		}
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
