package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileConfig holds security configuration for file tools
type FileConfig struct {
	Enabled       bool
	WorkspacePath string
}

// DefaultFileConfig returns the default file tool configuration
func DefaultFileConfig() *FileConfig {
	return &FileConfig{
		Enabled:       true, // Safer than exec, enabled by default
		WorkspacePath: "",
	}
}

// RegisterFileTools registers file_read, file_write, file_list tools
func RegisterFileTools(r *Registry, cfg *FileConfig) {
	if cfg == nil || !cfg.Enabled {
		return
	}

	r.Register(fileReadTool(cfg))
	r.Register(fileWriteTool(cfg))
	r.Register(fileListTool(cfg))
}

// validateFilePath checks if a path is safe (within workspace, no traversal)
func validateFilePath(workspacePath, requestedPath string) (string, error) {
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path not configured")
	}

	// Get absolute workspace path
	absWorkspace, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", fmt.Errorf("invalid workspace path: %w", err)
	}

	// Block absolute paths immediately (they bypass filepath.Join)
	if filepath.IsAbs(requestedPath) {
		return "", fmt.Errorf("path traversal denied: absolute paths not allowed")
	}

	// Join and clean the requested path
	fullPath := filepath.Join(absWorkspace, requestedPath)
	cleanPath := filepath.Clean(fullPath)

	// Get absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}

	// Ensure the resolved path is within the workspace
	if !strings.HasPrefix(absPath, absWorkspace+string(filepath.Separator)) && absPath != absWorkspace {
		return "", fmt.Errorf("path traversal denied: path must be within workspace")
	}

	return absPath, nil
}

// fileReadTool creates the file_read tool
func fileReadTool(cfg *FileConfig) *Tool {
	return &Tool{
		Name:        "file_read",
		Description: "Read a file from the workspace. Returns the file contents. Maximum 100KB.",
		Parameters: []Parameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "File path relative to workspace (e.g., 'notes.txt' or 'subdir/file.md')",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			path, ok := params["path"].(string)
			if !ok || path == "" {
				return "", fmt.Errorf("path parameter is required")
			}

			absPath, err := validateFilePath(cfg.WorkspacePath, path)
			if err != nil {
				return "", err
			}

			// Check file info
			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("file not found: %s", path)
				}
				return "", fmt.Errorf("cannot access file: %w", err)
			}

			if info.IsDir() {
				return "", fmt.Errorf("path is a directory, use file_list instead")
			}

			// Limit file size
			const maxSize = 100 * 1024 // 100KB
			if info.Size() > maxSize {
				return "", fmt.Errorf("file too large (%.1fKB > 100KB limit)", float64(info.Size())/1024)
			}

			content, err := os.ReadFile(absPath)
			if err != nil {
				return "", fmt.Errorf("failed to read file: %w", err)
			}

			return string(content), nil
		},
	}
}

// fileWriteTool creates the file_write tool
func fileWriteTool(cfg *FileConfig) *Tool {
	return &Tool{
		Name:        "file_write",
		Description: "Write content to a file in the workspace. Creates the file if it doesn't exist, overwrites if it does. Creates parent directories automatically.",
		Parameters: []Parameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "File path relative to workspace (e.g., 'notes.txt' or 'subdir/file.md')",
				Required:    true,
			},
			{
				Name:        "content",
				Type:        "string",
				Description: "Content to write to the file",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			path, ok := params["path"].(string)
			if !ok || path == "" {
				return "", fmt.Errorf("path parameter is required")
			}

			content, ok := params["content"].(string)
			if !ok {
				return "", fmt.Errorf("content parameter is required")
			}

			absPath, err := validateFilePath(cfg.WorkspacePath, path)
			if err != nil {
				return "", err
			}

			// Create parent directories if needed
			dir := filepath.Dir(absPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", fmt.Errorf("failed to create directories: %w", err)
			}

			// Write file
			if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
				return "", fmt.Errorf("failed to write file: %w", err)
			}

			return fmt.Sprintf("✅ File written: %s (%d bytes)", path, len(content)), nil
		},
	}
}

// fileListTool creates the file_list tool
func fileListTool(cfg *FileConfig) *Tool {
	return &Tool{
		Name:        "file_list",
		Description: "List files in a workspace directory. Returns file names with sizes.",
		Parameters: []Parameter{
			{
				Name:        "path",
				Type:        "string",
				Description: "Directory path relative to workspace (e.g., '.' for root, 'subdir' for subdirectory)",
				Required:    false,
			},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			path := "."
			if p, ok := params["path"].(string); ok && p != "" {
				path = p
			}

			absPath, err := validateFilePath(cfg.WorkspacePath, path)
			if err != nil {
				return "", err
			}

			// Check if path exists and is a directory
			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					return "", fmt.Errorf("directory not found: %s", path)
				}
				return "", fmt.Errorf("cannot access directory: %w", err)
			}

			if !info.IsDir() {
				return "", fmt.Errorf("path is not a directory")
			}

			// Read directory
			entries, err := os.ReadDir(absPath)
			if err != nil {
				return "", fmt.Errorf("failed to read directory: %w", err)
			}

			if len(entries) == 0 {
				return fmt.Sprintf("📂 %s/ (empty)", path), nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📂 %s/\n\n", path))

			for _, entry := range entries {
				info, err := entry.Info()
				if err != nil {
					continue
				}

				if entry.IsDir() {
					sb.WriteString(fmt.Sprintf("📁 %s/\n", entry.Name()))
				} else {
					size := formatFileSize(info.Size())
					sb.WriteString(fmt.Sprintf("📄 %s (%s)\n", entry.Name(), size))
				}
			}

			return sb.String(), nil
		},
	}
}

// formatFileSize returns a human-readable file size
func formatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
