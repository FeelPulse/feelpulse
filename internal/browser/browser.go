// Package browser provides browser automation tools for AI agents using go-rod.
package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// Config holds browser configuration
type Config struct {
	Enabled        bool `yaml:"enabled"`
	Headless       bool `yaml:"headless"`
	TimeoutSeconds int  `yaml:"timeoutSeconds"`
	Stealth        bool `yaml:"stealth"`
}

// DefaultConfig returns sensible defaults for browser config
func DefaultConfig() *Config {
	return &Config{
		Enabled:        true,
		Headless:       true,
		TimeoutSeconds: 30,
		Stealth:        true,
	}
}

// GetTimeout returns the timeout as time.Duration
func (c *Config) GetTimeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

// Browser manages browser automation with go-rod
type Browser struct {
	rod      *rod.Browser
	timeout  time.Duration
	headless bool
	stealth  bool

	// Callback for screenshot notifications (e.g., to send to Telegram)
	OnScreenshot func(path string) error
}

// ToolDefinition describes a tool that can be called by the AI
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// New creates a new Browser instance.
// Returns error if Chrome/Chromium is not found (graceful degradation).
func New(cfg *Config) (*Browser, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Try to find Chrome/Chromium
	path, found := launcher.LookPath()
	if !found {
		return nil, errors.New("Chrome/Chromium not found - browser tools disabled")
	}

	// Create launcher with configuration
	l := launcher.New().
		Bin(path).
		Headless(cfg.Headless).
		Set("disable-gpu").
		Set("no-sandbox").
		Set("disable-dev-shm-usage")

	// Launch browser
	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	// Connect to browser
	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to browser: %w", err)
	}

	return &Browser{
		rod:      browser,
		timeout:  cfg.GetTimeout(),
		headless: cfg.Headless,
		stealth:  cfg.Stealth,
	}, nil
}

// Close shuts down the browser
func (b *Browser) Close() {
	if b.rod != nil {
		_ = b.rod.Close()
	}
}

// SetScreenshotCallback sets the callback for screenshot notifications
func (b *Browser) SetScreenshotCallback(cb func(path string) error) {
	b.OnScreenshot = cb
}

// ToolDefinitions returns the AI tool definitions for all browser tools
func ToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "browser_navigate",
			Description: "Open a URL in the browser and return the page title and full text content (cleaned, no HTML tags)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to navigate to",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "browser_screenshot",
			Description: "Take a screenshot of a web page, save it to a file, and return the file path",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to screenshot",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "Optional CSS selector to screenshot a specific element",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "browser_click",
			Description: "Navigate to a URL and click an element by CSS selector",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to navigate to",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector of the element to click",
					},
				},
				"required": []string{"url", "selector"},
			},
		},
		{
			Name:        "browser_fill",
			Description: "Fill form fields on a page and optionally submit the form",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL with the form",
					},
					"fields": map[string]interface{}{
						"type":        "object",
						"description": "Object mapping CSS selectors to values to fill",
					},
					"submit": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to submit the form after filling",
					},
				},
				"required": []string{"url", "fields"},
			},
		},
		{
			Name:        "browser_extract",
			Description: "Extract specific data from a page using CSS selectors",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to extract data from",
					},
					"selector": map[string]interface{}{
						"type":        "string",
						"description": "CSS selector to find elements",
					},
					"attribute": map[string]interface{}{
						"type":        "string",
						"description": "Optional attribute to extract (e.g., 'href', 'src'). Default: text content",
					},
				},
				"required": []string{"url", "selector"},
			},
		},
		{
			Name:        "browser_script",
			Description: "Execute JavaScript on a page and return the result",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL to execute the script on",
					},
					"script": map[string]interface{}{
						"type":        "string",
						"description": "JavaScript code to execute (use 'return' to return a value)",
					},
				},
				"required": []string{"url", "script"},
			},
		},
	}
}

// ExecuteTool executes a browser tool by name with given parameters
func (b *Browser) ExecuteTool(name string, params map[string]interface{}) (string, error) {
	switch name {
	case "browser_navigate":
		return b.Navigate(params)
	case "browser_screenshot":
		return b.Screenshot(params)
	case "browser_click":
		return b.Click(params)
	case "browser_fill":
		return b.Fill(params)
	case "browser_extract":
		return b.Extract(params)
	case "browser_script":
		return b.Script(params)
	default:
		return "", fmt.Errorf("unknown browser tool: %s", name)
	}
}

// Navigate opens a URL and returns the page title and text content
func (b *Browser) Navigate(params map[string]interface{}) (string, error) {
	urlStr, err := parseNavigateParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Get page title
	title, err := page.MustInfo().Title, nil
	if err != nil {
		title = "Unknown"
	}

	// Get text content
	text, err := page.MustElement("body").Text()
	if err != nil {
		text = "(failed to extract text)"
	}

	text = cleanText(text)

	return fmt.Sprintf("title: %s\n\ncontent: %s", title, text), nil
}

// Screenshot takes a screenshot of a URL and returns the file path
func (b *Browser) Screenshot(params map[string]interface{}) (string, error) {
	urlStr, selector, err := parseScreenshotParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Create temp file for screenshot
	tmpFile, err := os.CreateTemp("", "screenshot-*.png")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFile.Close()
	path := tmpFile.Name()

	var data []byte
	if selector != "" {
		// Screenshot specific element
		el, err := page.Element(selector)
		if err != nil {
			return "", fmt.Errorf("element not found: %s", selector)
		}
		data, err = el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
		if err != nil {
			return "", fmt.Errorf("element screenshot failed: %w", err)
		}
	} else {
		// Full page screenshot
		data, err = page.Screenshot(true, &proto.PageCaptureScreenshot{
			Format:  proto.PageCaptureScreenshotFormatPng,
			Quality: nil,
		})
		if err != nil {
			return "", fmt.Errorf("screenshot failed: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write screenshot: %w", err)
	}

	// Notify callback if set (e.g., to send to Telegram)
	if b.OnScreenshot != nil {
		if err := b.OnScreenshot(path); err != nil {
			// Log but don't fail - screenshot was still taken
			fmt.Printf("⚠️ Screenshot callback failed: %v\n", err)
		}
	}

	return path, nil
}

// Click navigates to a URL and clicks an element
func (b *Browser) Click(params map[string]interface{}) (string, error) {
	urlStr, selector, err := parseClickParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Find and click element
	el, err := page.Element(selector)
	if err != nil {
		return "", fmt.Errorf("element not found: %s", selector)
	}

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	// Wait a moment for navigation
	time.Sleep(500 * time.Millisecond)

	// Get new URL
	newURL := page.MustInfo().URL

	return fmt.Sprintf("clicked %s, new URL: %s", selector, newURL), nil
}

// Fill fills form fields and optionally submits
func (b *Browser) Fill(params map[string]interface{}) (string, error) {
	urlStr, fields, submit, err := parseFillParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Fill each field
	filled := 0
	for selector, value := range fields {
		el, err := page.Element(selector)
		if err != nil {
			return "", fmt.Errorf("field not found: %s", selector)
		}

		if err := el.SelectAllText(); err == nil {
			// Clear existing text
			el.MustInput("")
		}
		if err := el.Input(value); err != nil {
			return "", fmt.Errorf("failed to fill %s: %w", selector, err)
		}
		filled++
	}

	if submit {
		// Find and click submit button (common patterns)
		submitSelectors := []string{
			"button[type='submit']",
			"input[type='submit']",
			"button:not([type])",
			"form button",
		}

		var submitted bool
		for _, sel := range submitSelectors {
			if el, err := page.Element(sel); err == nil {
				if err := el.Click(proto.InputMouseButtonLeft, 1); err == nil {
					submitted = true
					break
				}
			}
		}

		if submitted {
			// Wait for navigation
			time.Sleep(1 * time.Second)
			newURL := page.MustInfo().URL
			return fmt.Sprintf("submitted form, new URL: %s", newURL), nil
		}
		return fmt.Sprintf("filled %d fields, but couldn't find submit button", filled), nil
	}

	return fmt.Sprintf("filled %d fields", filled), nil
}

// Extract extracts data from elements using CSS selectors
func (b *Browser) Extract(params map[string]interface{}) (string, error) {
	urlStr, selector, attribute, err := parseExtractParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Find all matching elements
	elements, err := page.Elements(selector)
	if err != nil {
		return "", fmt.Errorf("selector error: %w", err)
	}

	values := make([]string, 0, len(elements))
	for _, el := range elements {
		var value string
		if attribute != "" {
			val, err := el.Attribute(attribute)
			if err == nil && val != nil {
				value = *val
			}
		} else {
			text, err := el.Text()
			if err == nil {
				value = strings.TrimSpace(text)
			}
		}
		if value != "" {
			values = append(values, value)
		}
	}

	// Return as JSON array
	data, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to serialize results: %w", err)
	}

	return string(data), nil
}

// Script executes JavaScript on a page and returns the result
func (b *Browser) Script(params map[string]interface{}) (string, error) {
	urlStr, script, err := parseScriptParams(params)
	if err != nil {
		return "", err
	}

	page, err := b.newPage(urlStr)
	if err != nil {
		return "", err
	}
	defer page.Close()

	// Wait for page to load
	if err := page.WaitLoad(); err != nil {
		return "", fmt.Errorf("page load timeout: %w", err)
	}

	// Execute script
	result, err := page.Eval(script)
	if err != nil {
		return "", fmt.Errorf("script error: %w", err)
	}

	// Return JSON-stringified result
	data, err := json.Marshal(result.Value)
	if err != nil {
		return "", fmt.Errorf("failed to serialize result: %w", err)
	}

	return string(data), nil
}

// newPage creates a new page with timeout and stealth mode
func (b *Browser) newPage(urlStr string) (*rod.Page, error) {
	if err := validateURL(urlStr); err != nil {
		return nil, err
	}

	// Create blank page first
	page, err := b.rod.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	// Apply stealth mode if enabled - inject before navigation
	if b.stealth {
		if _, err := page.EvalOnNewDocument(stealth.JS); err != nil {
			page.Close()
			return nil, fmt.Errorf("failed to inject stealth: %w", err)
		}
	}

	// Set timeout
	page = page.Timeout(b.timeout)

	// Navigate to the URL
	if err := page.Navigate(urlStr); err != nil {
		page.Close()
		return nil, fmt.Errorf("failed to navigate: %w", err)
	}

	return page, nil
}

// ============ Helper functions ============

// validateURL checks if a URL is valid and safe to navigate to
func validateURL(urlStr string) error {
	if urlStr == "" {
		return errors.New("URL is required")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s (only http/https allowed)", u.Scheme)
	}

	// Block dangerous schemes that might be embedded
	lower := strings.ToLower(urlStr)
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "file:") || strings.Contains(lower, "data:") {
		return errors.New("dangerous URL scheme detected")
	}

	return nil
}

// cleanText normalizes text by collapsing whitespace
func cleanText(text string) string {
	// Replace tabs with spaces
	text = strings.ReplaceAll(text, "\t", " ")

	// Collapse multiple spaces into one
	spaceRe := regexp.MustCompile(` {2,}`)
	text = spaceRe.ReplaceAllString(text, " ")

	// Collapse multiple newlines into max 2
	newlineRe := regexp.MustCompile(`\n{3,}`)
	text = newlineRe.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// parseNavigateParams extracts URL from params
func parseNavigateParams(params map[string]interface{}) (string, error) {
	urlVal, ok := params["url"]
	if !ok {
		return "", errors.New("url parameter is required")
	}
	urlStr, ok := urlVal.(string)
	if !ok || urlStr == "" {
		return "", errors.New("url must be a non-empty string")
	}
	if err := validateURL(urlStr); err != nil {
		return "", err
	}
	return urlStr, nil
}

// parseScreenshotParams extracts URL and optional selector
func parseScreenshotParams(params map[string]interface{}) (url, selector string, err error) {
	url, err = parseNavigateParams(params)
	if err != nil {
		return "", "", err
	}

	if sel, ok := params["selector"].(string); ok {
		selector = sel
	}

	return url, selector, nil
}

// parseClickParams extracts URL and selector
func parseClickParams(params map[string]interface{}) (url, selector string, err error) {
	url, err = parseNavigateParams(params)
	if err != nil {
		return "", "", err
	}

	selVal, ok := params["selector"]
	if !ok {
		return "", "", errors.New("selector parameter is required")
	}
	selector, ok = selVal.(string)
	if !ok || selector == "" {
		return "", "", errors.New("selector must be a non-empty string")
	}

	return url, selector, nil
}

// parseFillParams extracts URL, fields map, and submit flag
func parseFillParams(params map[string]interface{}) (url string, fields map[string]string, submit bool, err error) {
	url, err = parseNavigateParams(params)
	if err != nil {
		return "", nil, false, err
	}

	fieldsVal, ok := params["fields"]
	if !ok {
		return "", nil, false, errors.New("fields parameter is required")
	}

	fieldsMap, ok := fieldsVal.(map[string]interface{})
	if !ok {
		return "", nil, false, errors.New("fields must be an object")
	}

	fields = make(map[string]string)
	for k, v := range fieldsMap {
		if str, ok := v.(string); ok {
			fields[k] = str
		} else {
			fields[k] = fmt.Sprintf("%v", v)
		}
	}

	if len(fields) == 0 {
		return "", nil, false, errors.New("fields cannot be empty")
	}

	if submitVal, ok := params["submit"].(bool); ok {
		submit = submitVal
	}

	return url, fields, submit, nil
}

// parseExtractParams extracts URL, selector, and optional attribute
func parseExtractParams(params map[string]interface{}) (url, selector, attribute string, err error) {
	url, err = parseNavigateParams(params)
	if err != nil {
		return "", "", "", err
	}

	selVal, ok := params["selector"]
	if !ok {
		return "", "", "", errors.New("selector parameter is required")
	}
	selector, ok = selVal.(string)
	if !ok || selector == "" {
		return "", "", "", errors.New("selector must be a non-empty string")
	}

	if attr, ok := params["attribute"].(string); ok {
		attribute = attr
	}

	return url, selector, attribute, nil
}

// parseScriptParams extracts URL and script
func parseScriptParams(params map[string]interface{}) (url, script string, err error) {
	url, err = parseNavigateParams(params)
	if err != nil {
		return "", "", err
	}

	scriptVal, ok := params["script"]
	if !ok {
		return "", "", errors.New("script parameter is required")
	}
	script, ok = scriptVal.(string)
	if !ok || script == "" {
		return "", "", errors.New("script must be a non-empty string")
	}

	return url, script, nil
}

// IsBrowserTool checks if a tool name is a browser tool
func IsBrowserTool(name string) bool {
	return strings.HasPrefix(name, "browser_")
}

// GetScreenshotPath extracts the screenshot path from a tool result
// Returns empty string if result is not a valid screenshot path
func GetScreenshotPath(result string) string {
	// Screenshot results are just the path
	if strings.HasPrefix(result, "/") && strings.HasSuffix(result, ".png") {
		return result
	}
	// Also check temp dir paths on different systems
	if strings.Contains(result, "screenshot-") && strings.HasSuffix(result, ".png") {
		return result
	}
	return ""
}

// TempDir returns the path to the browser temp directory for screenshots
func TempDir() string {
	return filepath.Join(os.TempDir(), "feelpulse-browser")
}
