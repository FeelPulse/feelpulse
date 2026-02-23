package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ExecConfig holds security configuration for the exec tool
type ExecConfig struct {
	Enabled         bool
	AllowedCommands []string
	TimeoutSeconds  int
}

// DefaultExecConfig returns the default (disabled) exec configuration
func DefaultExecConfig() *ExecConfig {
	return &ExecConfig{
		Enabled:         false,
		AllowedCommands: []string{},
		TimeoutSeconds:  30,
	}
}

// dangerousPatterns are blocked regardless of allowlist
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+(-rf|-fr|--recursive)?\s*(\/|~|\$HOME|\$PWD|\*)`),          // rm -rf dangerous paths
	regexp.MustCompile(`(?i)\bsudo\s+(rm|chmod|chown|mkfs|dd|reboot|shutdown|halt|poweroff)\b`), // dangerous sudo commands
	regexp.MustCompile(`(?i)\bsu\s+-`),                                                          // su -
	regexp.MustCompile(`\.\./`),                                                                 // path traversal
	regexp.MustCompile(`(?i)\b(curl|wget)\b.*\|\s*(ba)?sh`),                                    // curl | sh
	regexp.MustCompile(`(?i)\bchmod\s+(777|[+]?[aou][+][rwx]s)`),                              // chmod dangerous perms
	regexp.MustCompile(`(?i)\bdd\s+.*of=/dev/`),                                                // dd to device
	regexp.MustCompile(`(?i)\bmkfs\b`),                                                         // filesystem format
	regexp.MustCompile(`(?i)\b(reboot|shutdown|halt|poweroff)\b`),                              // system control
	regexp.MustCompile(`(?i)>[>&]?\s*/etc/`),                                                   // writing to /etc
	regexp.MustCompile(`(?i)>[>&]?\s*/dev/`),                                                   // writing to /dev
}

// RegisterBuiltins registers all built-in tools
func RegisterBuiltins(r *Registry) {
	// Note: exec tool is NOT registered by default for security
	// Use RegisterBuiltinsWithExec to enable it with proper configuration
	r.Register(webSearchTool())
}

// RegisterBuiltinsWithExec registers built-in tools including exec with security config
func RegisterBuiltinsWithExec(r *Registry, execCfg *ExecConfig) {
	r.Register(webSearchTool())

	// Only register exec if explicitly enabled
	if execCfg != nil && execCfg.Enabled {
		r.Register(execToolSecure(execCfg))
	}
}

// BrowserToolRegistrar is an interface for browser tool registration
// This allows the browser package to register its tools without import cycles
type BrowserToolRegistrar interface {
	RegisterBrowserTools(r *Registry)
}

// shellQuote quotes a string for safe use as a shell argument
func shellQuote(s string) string {
	// Simple quoting: wrap in single quotes and escape any single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// validateExecCommand checks if a command is allowed
func validateExecCommand(cmdStr string, allowedCommands []string) error {
	// First check for dangerous patterns (blocked regardless of allowlist)
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cmdStr) {
			return fmt.Errorf("command contains blocked pattern: security policy violation")
		}
	}

	// If no allowlist configured, deny all
	if len(allowedCommands) == 0 {
		return fmt.Errorf("exec tool has no allowed commands configured")
	}

	// Extract the first word (command name) from the command string
	cmdStr = strings.TrimSpace(cmdStr)
	cmdParts := strings.Fields(cmdStr)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	cmdName := cmdParts[0]

	// Check if command is in allowlist
	for _, allowed := range allowedCommands {
		// Support prefix matching (e.g., "git" allows "git status", "git log", etc.)
		if cmdName == allowed || strings.HasPrefix(cmdName, allowed+"/") {
			return nil
		}
	}

	return fmt.Errorf("command '%s' is not in the allowed commands list", cmdName)
}

// execToolSecure creates the exec tool with security controls
func execToolSecure(cfg *ExecConfig) *Tool {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Tool{
		Name:        "exec",
		Description: "Execute a shell command and return the output. Only allowed commands can be run (configured by admin).",
		Parameters: []Parameter{
			{
				Name:        "command",
				Type:        "string",
				Description: "The shell command to execute (must be in allowed commands list)",
				Required:    true,
			},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			cmdStr, ok := params["command"].(string)
			if !ok || cmdStr == "" {
				return "", fmt.Errorf("command parameter is required")
			}

			// Auto-wrap with bash if "bash" is the only allowed command
			// This allows "git clone", "python3", etc. to work when allowedCommands=["bash"]
			if len(cfg.AllowedCommands) == 1 && cfg.AllowedCommands[0] == "bash" {
				if !strings.HasPrefix(strings.TrimSpace(cmdStr), "bash ") {
					// Wrap the command in bash -c
					cmdStr = "bash -c " + shellQuote(cmdStr)
				}
			}

			// Validate command against security policy
			if err := validateExecCommand(cmdStr, cfg.AllowedCommands); err != nil {
				return "", err
			}

			// Create context with timeout
			execCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Create command
			cmd := exec.CommandContext(execCtx, "sh", "-c", cmdStr)

			// Prevent interactive prompts (git credentials, ssh, sudo passwords, etc.)
			// Inherit environment but override prompt-related vars
			cmd.Env = append(os.Environ(),
				"GIT_TERMINAL_PROMPT=0",   // git: fail instead of prompting for credentials
				"GIT_ASKPASS=echo",        // git: return empty string for any credential ask
				"SSH_ASKPASS=echo",        // ssh: return empty string for passphrase
				"DEBIAN_FRONTEND=noninteractive", // apt: non-interactive mode
				"SUDO_ASKPASS=echo",       // sudo -A: return empty (note: regular sudo still prompts via tty)
			)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			result := stdout.String()
			if stderr.Len() > 0 {
				if result != "" {
					result += "\n"
				}
				result += "stderr: " + stderr.String()
			}

			if err != nil {
				if execCtx.Err() == context.DeadlineExceeded {
					return "", fmt.Errorf("command timed out after %v", timeout)
				}
				if result == "" {
					return "", fmt.Errorf("command failed: %v", err)
				}
				// Return output even if command failed (e.g., non-zero exit)
				result += fmt.Sprintf("\n(exit error: %v)", err)
			}

			// Truncate very long output
			const maxLen = 10000
			if len(result) > maxLen {
				result = result[:maxLen] + "\n... (output truncated)"
			}

			return strings.TrimSpace(result), nil
		},
	}
}

// execTool creates the exec tool for running shell commands (DEPRECATED - use execToolSecure)
// Kept for backward compatibility but should not be used directly
func execTool() *Tool {
	return execToolSecure(&ExecConfig{
		Enabled:         true,
		AllowedCommands: []string{}, // Empty = deny all
		TimeoutSeconds:  30,
	})
}

// webSearchTool creates the web search tool
func webSearchTool() *Tool {
	return &Tool{
		Name:        "web_search",
		Description: "Search the web for information using DuckDuckGo. Returns relevant search results with titles, URLs, and snippets. Also provides instant answers for simple queries.",
		Parameters: []Parameter{
			{
				Name:        "query",
				Type:        "string",
				Description: "The search query",
				Required:    true,
			},
			{
				Name:        "limit",
				Type:        "integer",
				Description: "Maximum number of results to return (default: 5)",
				Required:    false,
			},
		},
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			query, ok := params["query"].(string)
			if !ok || query == "" {
				return "", fmt.Errorf("query parameter is required")
			}

			limit := 5
			if l, ok := params["limit"].(float64); ok {
				limit = int(l)
			} else if l, ok := params["limit"].(int); ok {
				limit = l
			}
			if limit < 1 {
				limit = 1
			}
			if limit > 10 {
				limit = 10
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Search results for '%s':\n\n", query))

			// Try instant answer API first for quick facts
			instant, err := duckDuckGoInstantAnswer(ctx, query)
			if err == nil && instant != "" {
				sb.WriteString("📋 *Instant Answer:*\n")
				sb.WriteString(instant)
				sb.WriteString("\n\n")
			}

			// Get regular search results
			results, err := duckDuckGoSearch(ctx, query, limit)
			if err != nil {
				// If instant answer succeeded, return that even if search fails
				if instant != "" {
					sb.WriteString("(Web search unavailable: " + err.Error() + ")\n")
					return sb.String(), nil
				}
				return "", fmt.Errorf("search failed: %w", err)
			}

			if len(results) == 0 && instant == "" {
				return "No results found for: " + query, nil
			}

			if len(results) > 0 {
				sb.WriteString("🔍 *Web Results:*\n\n")
				for i, r := range results {
					sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
					sb.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
					if r.Snippet != "" {
						sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
					}
					sb.WriteString("\n")
				}
			}

			return sb.String(), nil
		},
	}
}

// duckDuckGoInstantAnswer fetches instant answers from DuckDuckGo's API
func duckDuckGoInstantAnswer(ctx context.Context, query string) (string, error) {
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1",
		url.QueryEscape(query))

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var ddgResp DuckDuckGoResponse
	if err := json.NewDecoder(resp.Body).Decode(&ddgResp); err != nil {
		return "", err
	}

	// Build instant answer from available data
	var answer strings.Builder

	if ddgResp.AbstractText != "" {
		answer.WriteString(ddgResp.AbstractText)
		if ddgResp.AbstractSource != "" {
			answer.WriteString(fmt.Sprintf("\n(Source: %s)", ddgResp.AbstractSource))
		}
	}

	return answer.String(), nil
}

// SearchResult represents a web search result
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// DuckDuckGoResponse represents the DDG API response
type DuckDuckGoResponse struct {
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	AbstractSource string `json:"AbstractSource"`
	RelatedTopics  []struct {
		Text      string `json:"Text"`
		FirstURL  string `json:"FirstURL"`
		Result    string `json:"Result"`
	} `json:"RelatedTopics"`
	Results []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"Results"`
}

// duckDuckGoSearch performs a search using DuckDuckGo's Instant Answer API
func duckDuckGoSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Use DuckDuckGo's HTML search page since the API is limited
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use a realistic user agent to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("search request timed out")
		}
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limiting or blocking
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited by DuckDuckGo, please try again later")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("blocked by DuckDuckGo, search unavailable")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	// Limit response body size to avoid memory issues
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024)) // 512KB max
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Simple HTML parsing - extract links and titles
	results := parseSearchResults(string(body), limit)

	return results, nil
}

// parseSearchResults extracts results from DuckDuckGo HTML
func parseSearchResults(html string, limit int) []SearchResult {
	var results []SearchResult

	// Look for result links in the HTML
	// DDG uses class="result__a" for result links
	parts := strings.Split(html, `class="result__a"`)
	
	for i := 1; i < len(parts) && len(results) < limit; i++ {
		part := parts[i]

		// Extract href
		hrefStart := strings.Index(part, `href="`)
		if hrefStart == -1 {
			continue
		}
		hrefStart += 6
		hrefEnd := strings.Index(part[hrefStart:], `"`)
		if hrefEnd == -1 {
			continue
		}
		rawURL := part[hrefStart : hrefStart+hrefEnd]

		// DDG wraps URLs - extract actual URL
		actualURL := rawURL
		if strings.Contains(rawURL, "uddg=") {
			if idx := strings.Index(rawURL, "uddg="); idx != -1 {
				actualURL = rawURL[idx+5:]
				if end := strings.Index(actualURL, "&"); end != -1 {
					actualURL = actualURL[:end]
				}
				actualURL, _ = url.QueryUnescape(actualURL)
			}
		}

		// Extract title (text between > and <)
		titleStart := strings.Index(part[hrefStart:], ">")
		if titleStart == -1 {
			continue
		}
		titleStart += hrefStart + 1
		titleEnd := strings.Index(part[titleStart:], "<")
		if titleEnd == -1 {
			continue
		}
		title := strings.TrimSpace(part[titleStart : titleStart+titleEnd])

		// Extract snippet if available
		snippet := ""
		snippetPart := parts[i]
		if snippetStart := strings.Index(snippetPart, `class="result__snippet"`); snippetStart != -1 {
			snippetTextStart := strings.Index(snippetPart[snippetStart:], ">")
			if snippetTextStart != -1 {
				snippetTextStart += snippetStart + 1
				snippetEnd := strings.Index(snippetPart[snippetTextStart:], "<")
				if snippetEnd != -1 {
					snippet = strings.TrimSpace(snippetPart[snippetTextStart : snippetTextStart+snippetEnd])
				}
			}
		}

		if title != "" && actualURL != "" {
			results = append(results, SearchResult{
				Title:   cleanHTML(title),
				URL:     actualURL,
				Snippet: cleanHTML(snippet),
			})
		}
	}

	return results
}

// cleanHTML removes HTML tags and entities
func cleanHTML(s string) string {
	// Remove HTML tags
	for {
		start := strings.Index(s, "<")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], ">")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}

	// Decode common HTML entities
	replacements := map[string]string{
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": "\"",
		"&#39;":  "'",
		"&nbsp;": " ",
	}
	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
	}

	return strings.TrimSpace(s)
}

// BraveSearchResponse for future Brave Search API support
type BraveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// braveSearch performs a search using Brave Search API (requires API key)
func braveSearch(ctx context.Context, apiKey, query string, limit int) ([]SearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Brave API key not configured")
	}

	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), limit)

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Brave API error: %d - %s", resp.StatusCode, string(body))
	}

	var braveResp BraveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		})
	}

	return results, nil
}
