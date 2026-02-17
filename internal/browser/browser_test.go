package browser

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Unit tests - no real browser needed

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://example.com", false},
		{"https://example.com/path?query=1", false},
		{"ftp://example.com", true},       // unsupported scheme
		{"not-a-url", true},               // invalid URL
		{"", true},                         // empty
		{"javascript:alert(1)", true},     // dangerous scheme
		{"file:///etc/passwd", true},      // local file access
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := validateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple text",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "extra whitespace",
			input: "Hello   World\n\n\nTest",
			want:  "Hello World\n\nTest",
		},
		{
			name:  "trim spaces",
			input: "   Hello World   ",
			want:  "Hello World",
		},
		{
			name:  "tabs to spaces",
			input: "Hello\tWorld",
			want:  "Hello World",
		},
		{
			name:  "empty lines",
			input: "Line1\n\n\n\n\nLine2",
			want:  "Line1\n\nLine2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanText(tt.input)
			if got != tt.want {
				t.Errorf("cleanText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNavigateParams(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]interface{}
		wantURL string
		wantErr bool
	}{
		{
			name:    "valid URL",
			params:  map[string]interface{}{"url": "https://example.com"},
			wantURL: "https://example.com",
			wantErr: false,
		},
		{
			name:    "missing URL",
			params:  map[string]interface{}{},
			wantURL: "",
			wantErr: true,
		},
		{
			name:    "empty URL",
			params:  map[string]interface{}{"url": ""},
			wantURL: "",
			wantErr: true,
		},
		{
			name:    "invalid URL type",
			params:  map[string]interface{}{"url": 123},
			wantURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, err := parseNavigateParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNavigateParams() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotURL != tt.wantURL {
				t.Errorf("parseNavigateParams() url = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

func TestScreenshotParams(t *testing.T) {
	tests := []struct {
		name         string
		params       map[string]interface{}
		wantURL      string
		wantSelector string
		wantErr      bool
	}{
		{
			name:         "URL only",
			params:       map[string]interface{}{"url": "https://example.com"},
			wantURL:      "https://example.com",
			wantSelector: "",
			wantErr:      false,
		},
		{
			name:         "URL with selector",
			params:       map[string]interface{}{"url": "https://example.com", "selector": "#main"},
			wantURL:      "https://example.com",
			wantSelector: "#main",
			wantErr:      false,
		},
		{
			name:    "missing URL",
			params:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotSel, err := parseScreenshotParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScreenshotParams() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotURL != tt.wantURL {
				t.Errorf("parseScreenshotParams() url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotSel != tt.wantSelector {
				t.Errorf("parseScreenshotParams() selector = %q, want %q", gotSel, tt.wantSelector)
			}
		})
	}
}

func TestFillParams(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantURL    string
		wantFields map[string]string
		wantSubmit bool
		wantErr    bool
	}{
		{
			name: "basic fill",
			params: map[string]interface{}{
				"url":    "https://example.com",
				"fields": map[string]interface{}{"#name": "John"},
			},
			wantURL:    "https://example.com",
			wantFields: map[string]string{"#name": "John"},
			wantSubmit: false,
			wantErr:    false,
		},
		{
			name: "fill with submit",
			params: map[string]interface{}{
				"url":    "https://example.com",
				"fields": map[string]interface{}{"#email": "test@test.com"},
				"submit": true,
			},
			wantURL:    "https://example.com",
			wantFields: map[string]string{"#email": "test@test.com"},
			wantSubmit: true,
			wantErr:    false,
		},
		{
			name: "missing fields",
			params: map[string]interface{}{
				"url": "https://example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotFields, gotSubmit, err := parseFillParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFillParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if gotURL != tt.wantURL {
				t.Errorf("parseFillParams() url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotSubmit != tt.wantSubmit {
				t.Errorf("parseFillParams() submit = %v, want %v", gotSubmit, tt.wantSubmit)
			}
			for k, v := range tt.wantFields {
				if gotFields[k] != v {
					t.Errorf("parseFillParams() field %q = %q, want %q", k, gotFields[k], v)
				}
			}
		})
	}
}

func TestExtractParams(t *testing.T) {
	tests := []struct {
		name          string
		params        map[string]interface{}
		wantURL       string
		wantSelector  string
		wantAttribute string
		wantErr       bool
	}{
		{
			name:         "basic extract",
			params:       map[string]interface{}{"url": "https://example.com", "selector": "a"},
			wantURL:      "https://example.com",
			wantSelector: "a",
			wantErr:      false,
		},
		{
			name:          "extract with attribute",
			params:        map[string]interface{}{"url": "https://example.com", "selector": "a", "attribute": "href"},
			wantURL:       "https://example.com",
			wantSelector:  "a",
			wantAttribute: "href",
			wantErr:       false,
		},
		{
			name:    "missing selector",
			params:  map[string]interface{}{"url": "https://example.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotSel, gotAttr, err := parseExtractParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseExtractParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if gotURL != tt.wantURL {
				t.Errorf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotSel != tt.wantSelector {
				t.Errorf("selector = %q, want %q", gotSel, tt.wantSelector)
			}
			if gotAttr != tt.wantAttribute {
				t.Errorf("attribute = %q, want %q", gotAttr, tt.wantAttribute)
			}
		})
	}
}

func TestScriptParams(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantURL    string
		wantScript string
		wantErr    bool
	}{
		{
			name:       "basic script",
			params:     map[string]interface{}{"url": "https://example.com", "script": "return 1+1"},
			wantURL:    "https://example.com",
			wantScript: "return 1+1",
			wantErr:    false,
		},
		{
			name:    "missing script",
			params:  map[string]interface{}{"url": "https://example.com"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotScript, err := parseScriptParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScriptParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if gotURL != tt.wantURL {
				t.Errorf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotScript != tt.wantScript {
				t.Errorf("script = %q, want %q", gotScript, tt.wantScript)
			}
		})
	}
}

func TestToolDefinitions(t *testing.T) {
	tools := ToolDefinitions()

	// Should have 6 tools
	if len(tools) != 6 {
		t.Errorf("expected 6 tools, got %d", len(tools))
	}

	// Check tool names
	expectedNames := []string{
		"browser_navigate",
		"browser_screenshot",
		"browser_click",
		"browser_fill",
		"browser_extract",
		"browser_script",
	}

	for i, name := range expectedNames {
		if tools[i].Name != name {
			t.Errorf("tool %d: expected name %q, got %q", i, name, tools[i].Name)
		}
	}
}

func TestBrowserConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("expected browser to be enabled by default")
	}
	if !cfg.Headless {
		t.Error("expected headless to be true by default")
	}
	if cfg.TimeoutSeconds != 30 {
		t.Errorf("expected timeout 30, got %d", cfg.TimeoutSeconds)
	}
	if !cfg.Stealth {
		t.Error("expected stealth to be true by default")
	}
}

func TestBrowserTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TimeoutSeconds = 15

	timeout := cfg.GetTimeout()
	expected := 15 * time.Second

	if timeout != expected {
		t.Errorf("expected timeout %v, got %v", expected, timeout)
	}
}

// Test JSON serialization of extract results
func TestExtractResultsJSON(t *testing.T) {
	values := []string{"link1", "link2", "link3"}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	expected := `["link1","link2","link3"]`
	if string(data) != expected {
		t.Errorf("JSON = %s, want %s", data, expected)
	}
}

// Test that cleanText handles HTML entities properly (future enhancement)
func TestCleanTextComplex(t *testing.T) {
	// This tests our basic cleaning - real HTML stripping is done by rod
	input := "Hello     World\n\n\n\n\nParagraph 2\n\n\n\nParagraph 3"
	got := cleanText(input)

	// Should collapse multiple spaces and newlines
	if strings.Contains(got, "     ") {
		t.Error("failed to collapse multiple spaces")
	}
	if strings.Contains(got, "\n\n\n") {
		t.Error("failed to collapse multiple newlines")
	}
}

// Benchmark for text cleaning
func BenchmarkCleanText(b *testing.B) {
	input := strings.Repeat("Hello   World\n\n\nParagraph\t\tEnd  ", 100)
	for i := 0; i < b.N; i++ {
		cleanText(input)
	}
}
