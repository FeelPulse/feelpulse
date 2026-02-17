package gateway

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/config"
)

// ConfigPageData holds data for the config page template
type ConfigPageData struct {
	// Agent settings (secrets masked)
	AgentModel            string
	AgentMaxTokens        int
	AgentMaxContextTokens int
	AgentSystem           string
	AgentRateLimit        int
	AgentHasAPIKey        bool
	AgentHasAuthToken     bool

	// Tools > Exec
	ExecEnabled         bool
	ExecAllowedCommands string
	ExecTimeoutSeconds  int

	// Tools > File
	FileEnabled bool

	// Browser
	BrowserEnabled  bool
	BrowserHeadless bool

	// Heartbeat
	HeartbeatEnabled         bool
	HeartbeatIntervalMinutes int

	// Log
	LogLevel string

	// Hooks (token masked)
	HooksEnabled  bool
	HooksHasToken bool

	// Success/error messages
	Message  string
	Warnings []string
	IsError  bool
}

// handleConfigPage serves the configuration editor page
func (gw *Gateway) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	if !gw.checkAuth(w, r) {
		return
	}

	cfg, err := config.Load()
	if err != nil {
		http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := ConfigPageData{
		// Agent
		AgentModel:            cfg.Agent.Model,
		AgentMaxTokens:        cfg.Agent.MaxTokens,
		AgentMaxContextTokens: cfg.Agent.MaxContextTokens,
		AgentSystem:           cfg.Agent.System,
		AgentRateLimit:        cfg.Agent.RateLimit,
		AgentHasAPIKey:        cfg.Agent.APIKey != "",
		AgentHasAuthToken:     cfg.Agent.AuthToken != "",

		// Tools > Exec
		ExecEnabled:         cfg.Tools.Exec.Enabled,
		ExecAllowedCommands: strings.Join(cfg.Tools.Exec.AllowedCommands, ", "),
		ExecTimeoutSeconds:  cfg.Tools.Exec.TimeoutSeconds,

		// Tools > File
		FileEnabled: cfg.Tools.File.Enabled,

		// Browser
		BrowserEnabled:  cfg.Browser.Enabled,
		BrowserHeadless: cfg.Browser.Headless,

		// Heartbeat
		HeartbeatEnabled:         cfg.Heartbeat.Enabled,
		HeartbeatIntervalMinutes: cfg.Heartbeat.IntervalMinutes,

		// Log
		LogLevel: cfg.Log.Level,

		// Hooks
		HooksEnabled:  cfg.Hooks.Enabled,
		HooksHasToken: cfg.Hooks.Token != "",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(generateConfigPageHTML(data)))
}

// ConfigSaveRequest represents the JSON payload for saving config
type ConfigSaveRequest struct {
	// Agent
	AgentModel            string `json:"agentModel"`
	AgentMaxTokens        int    `json:"agentMaxTokens"`
	AgentMaxContextTokens int    `json:"agentMaxContextTokens"`
	AgentSystem           string `json:"agentSystem"`
	AgentRateLimit        int    `json:"agentRateLimit"`

	// Tools > Exec
	ExecEnabled         bool   `json:"execEnabled"`
	ExecAllowedCommands string `json:"execAllowedCommands"`
	ExecTimeoutSeconds  int    `json:"execTimeoutSeconds"`

	// Tools > File
	FileEnabled bool `json:"fileEnabled"`

	// Browser
	BrowserEnabled  bool `json:"browserEnabled"`
	BrowserHeadless bool `json:"browserHeadless"`

	// Heartbeat
	HeartbeatEnabled         bool `json:"heartbeatEnabled"`
	HeartbeatIntervalMinutes int  `json:"heartbeatIntervalMinutes"`

	// Log
	LogLevel string `json:"logLevel"`

	// Hooks
	HooksEnabled bool   `json:"hooksEnabled"`
	HooksToken   string `json:"hooksToken"`
}

// ConfigSaveResponse represents the JSON response after saving config
type ConfigSaveResponse struct {
	OK       bool     `json:"ok"`
	Message  string   `json:"message,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// handleConfigSave processes POST requests to save configuration
func (gw *Gateway) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !gw.checkAuth(w, r) {
		return
	}

	var req ConfigSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ConfigSaveResponse{
			OK:    false,
			Error: "Invalid JSON: " + err.Error(),
		})
		return
	}

	// Load existing config to preserve secrets and other settings
	cfg, err := config.Load()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ConfigSaveResponse{
			OK:    false,
			Error: "Failed to load existing config: " + err.Error(),
		})
		return
	}

	// Update only the editable fields (preserve secrets)
	cfg.Agent.Model = req.AgentModel
	cfg.Agent.MaxTokens = req.AgentMaxTokens
	cfg.Agent.MaxContextTokens = req.AgentMaxContextTokens
	cfg.Agent.System = req.AgentSystem
	cfg.Agent.RateLimit = req.AgentRateLimit

	cfg.Tools.Exec.Enabled = req.ExecEnabled
	cfg.Tools.Exec.AllowedCommands = parseCommaSeparated(req.ExecAllowedCommands)
	cfg.Tools.Exec.TimeoutSeconds = req.ExecTimeoutSeconds

	cfg.Tools.File.Enabled = req.FileEnabled

	cfg.Browser.Enabled = req.BrowserEnabled
	cfg.Browser.Headless = req.BrowserHeadless

	cfg.Heartbeat.Enabled = req.HeartbeatEnabled
	cfg.Heartbeat.IntervalMinutes = req.HeartbeatIntervalMinutes

	cfg.Log.Level = req.LogLevel

	cfg.Hooks.Enabled = req.HooksEnabled
	// Only update hooks token if a new value is provided (not empty)
	if req.HooksToken != "" {
		cfg.Hooks.Token = req.HooksToken
	}

	// Validate the config
	result := cfg.Validate()
	if !result.IsValid() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ConfigSaveResponse{
			OK:       false,
			Error:    strings.Join(result.Errors, "; "),
			Warnings: result.Warnings,
		})
		return
	}

	// Save the config
	_, err = config.Save(cfg)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ConfigSaveResponse{
			OK:    false,
			Error: "Failed to save config: " + err.Error(),
		})
		return
	}

	gw.log.Info("⚙️ Config saved via dashboard")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ConfigSaveResponse{
		OK:       true,
		Message:  "Config saved. Restart FeelPulse to apply changes.",
		Warnings: result.Warnings,
	})
}

// parseCommaSeparated parses a comma-separated string into a slice of trimmed strings
func parseCommaSeparated(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

const configPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FeelPulse Configuration</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #eee;
            min-height: 100vh;
            padding: 2rem;
        }
        .container { max-width: 900px; margin: 0 auto; }
        header {
            display: flex;
            align-items: center;
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .logo { font-size: 2.5rem; }
        h1 { font-size: 2rem; font-weight: 600; }
        .subtitle { color: #888; font-size: 0.9rem; margin-top: 0.25rem; }
        .nav-links { margin-left: auto; display: flex; gap: 1rem; }
        .nav-links a {
            color: #4ade80;
            text-decoration: none;
            font-size: 0.9rem;
        }
        .nav-links a:hover { text-decoration: underline; }
        .card {
            background: rgba(255,255,255,0.05);
            border-radius: 12px;
            padding: 1.5rem;
            border: 1px solid rgba(255,255,255,0.1);
            margin-bottom: 1.5rem;
        }
        .card-title {
            font-size: 1.1rem;
            font-weight: 600;
            margin-bottom: 1rem;
            color: #fff;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .form-group {
            margin-bottom: 1rem;
        }
        .form-group:last-child {
            margin-bottom: 0;
        }
        label {
            display: block;
            font-size: 0.85rem;
            color: #aaa;
            margin-bottom: 0.25rem;
        }
        input[type="text"],
        input[type="number"],
        textarea,
        select {
            width: 100%;
            padding: 0.6rem 0.8rem;
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 6px;
            background: rgba(0,0,0,0.3);
            color: #fff;
            font-size: 0.9rem;
        }
        input:focus, textarea:focus, select:focus {
            outline: none;
            border-color: #4ade80;
        }
        textarea {
            min-height: 80px;
            resize: vertical;
        }
        .toggle-row {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }
        .toggle {
            position: relative;
            width: 44px;
            height: 24px;
        }
        .toggle input {
            opacity: 0;
            width: 0;
            height: 0;
        }
        .toggle-slider {
            position: absolute;
            cursor: pointer;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: #374151;
            border-radius: 24px;
            transition: 0.3s;
        }
        .toggle-slider:before {
            position: absolute;
            content: "";
            height: 18px;
            width: 18px;
            left: 3px;
            bottom: 3px;
            background: white;
            border-radius: 50%;
            transition: 0.3s;
        }
        .toggle input:checked + .toggle-slider {
            background: #4ade80;
        }
        .toggle input:checked + .toggle-slider:before {
            transform: translateX(20px);
        }
        .toggle-label {
            font-size: 0.9rem;
            color: #ddd;
        }
        .form-row {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1rem;
        }
        @media (max-width: 600px) {
            .form-row { grid-template-columns: 1fr; }
        }
        .secret-badge {
            display: inline-block;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.75rem;
            margin-left: 0.5rem;
        }
        .secret-badge.set { background: #166534; color: #4ade80; }
        .secret-badge.not-set { background: #7f1d1d; color: #fca5a5; }
        .hint {
            font-size: 0.75rem;
            color: #666;
            margin-top: 0.25rem;
        }
        .btn {
            padding: 0.75rem 1.5rem;
            border: none;
            border-radius: 8px;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
        }
        .btn-primary {
            background: #4ade80;
            color: #000;
        }
        .btn-primary:hover {
            background: #22c55e;
        }
        .btn-primary:disabled {
            background: #374151;
            color: #666;
            cursor: not-allowed;
        }
        .actions {
            display: flex;
            gap: 1rem;
            margin-top: 1.5rem;
        }
        .alert {
            padding: 1rem;
            border-radius: 8px;
            margin-bottom: 1.5rem;
        }
        .alert-success {
            background: rgba(74, 222, 128, 0.15);
            border: 1px solid #4ade80;
            color: #4ade80;
        }
        .alert-error {
            background: rgba(248, 113, 113, 0.15);
            border: 1px solid #f87171;
            color: #f87171;
        }
        .alert-warning {
            background: rgba(251, 191, 36, 0.15);
            border: 1px solid #fbbf24;
            color: #fbbf24;
        }
        .warning-list {
            margin-top: 0.5rem;
            padding-left: 1.5rem;
        }
        .warning-list li {
            margin-bottom: 0.25rem;
        }
        footer {
            margin-top: 2rem;
            text-align: center;
            color: #666;
            font-size: 0.8rem;
        }
        .spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #000;
            border-top-color: transparent;
            border-radius: 50%;
            animation: spin 0.8s linear infinite;
            margin-right: 0.5rem;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <span class="logo">⚙️</span>
            <div>
                <h1>Configuration</h1>
                <p class="subtitle">FeelPulse Settings</p>
            </div>
            <div class="nav-links">
                <a href="/dashboard">← Dashboard</a>
            </div>
        </header>

        <div id="alert-container"></div>

        <form id="config-form">
            <!-- Agent Settings -->
            <div class="card">
                <div class="card-title">
                    🤖 Agent
                    {{if .AgentHasAPIKey}}<span class="secret-badge set">API Key ✓</span>{{else}}<span class="secret-badge not-set">No API Key</span>{{end}}
                    {{if .AgentHasAuthToken}}<span class="secret-badge set">Auth Token ✓</span>{{else}}{{end}}
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="agentModel">Model</label>
                        <input type="text" id="agentModel" name="agentModel" value="{{.AgentModel}}" placeholder="claude-sonnet-4-20250514">
                    </div>
                    <div class="form-group">
                        <label for="agentRateLimit">Rate Limit (msg/min, 0=disabled)</label>
                        <input type="number" id="agentRateLimit" name="agentRateLimit" value="{{.AgentRateLimit}}" min="0">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label for="agentMaxTokens">Max Tokens</label>
                        <input type="number" id="agentMaxTokens" name="agentMaxTokens" value="{{.AgentMaxTokens}}" min="1">
                    </div>
                    <div class="form-group">
                        <label for="agentMaxContextTokens">Max Context Tokens</label>
                        <input type="number" id="agentMaxContextTokens" name="agentMaxContextTokens" value="{{.AgentMaxContextTokens}}" min="1000">
                    </div>
                </div>
                <div class="form-group">
                    <label for="agentSystem">System Prompt</label>
                    <textarea id="agentSystem" name="agentSystem" placeholder="Optional custom system prompt">{{.AgentSystem}}</textarea>
                    <div class="hint">Leave blank to use workspace SOUL.md</div>
                </div>
            </div>

            <!-- Tools > Exec -->
            <div class="card">
                <div class="card-title">🔧 Tools › Exec</div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="execEnabled" name="execEnabled" {{if .ExecEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Enable exec tool</span>
                    </div>
                </div>
                <div class="form-group">
                    <label for="execAllowedCommands">Allowed Commands (comma-separated)</label>
                    <input type="text" id="execAllowedCommands" name="execAllowedCommands" value="{{.ExecAllowedCommands}}" placeholder="ls, cat, echo, git">
                    <div class="hint">Leave empty to allow all commands (use with caution)</div>
                </div>
                <div class="form-group">
                    <label for="execTimeoutSeconds">Timeout (seconds)</label>
                    <input type="number" id="execTimeoutSeconds" name="execTimeoutSeconds" value="{{.ExecTimeoutSeconds}}" min="1" max="300">
                </div>
            </div>

            <!-- Tools > File -->
            <div class="card">
                <div class="card-title">📁 Tools › File</div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="fileEnabled" name="fileEnabled" {{if .FileEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Enable file tools (sandboxed to workspace)</span>
                    </div>
                </div>
            </div>

            <!-- Browser -->
            <div class="card">
                <div class="card-title">🌐 Browser</div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="browserEnabled" name="browserEnabled" {{if .BrowserEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Enable browser automation</span>
                    </div>
                </div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="browserHeadless" name="browserHeadless" {{if .BrowserHeadless}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Headless mode (no visible window)</span>
                    </div>
                </div>
            </div>

            <!-- Heartbeat -->
            <div class="card">
                <div class="card-title">💓 Heartbeat</div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="heartbeatEnabled" name="heartbeatEnabled" {{if .HeartbeatEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Enable heartbeat polling</span>
                    </div>
                </div>
                <div class="form-group">
                    <label for="heartbeatIntervalMinutes">Interval (minutes)</label>
                    <input type="number" id="heartbeatIntervalMinutes" name="heartbeatIntervalMinutes" value="{{.HeartbeatIntervalMinutes}}" min="1" max="1440">
                </div>
            </div>

            <!-- Log -->
            <div class="card">
                <div class="card-title">📝 Log</div>
                <div class="form-group">
                    <label for="logLevel">Log Level</label>
                    <select id="logLevel" name="logLevel">
                        <option value="debug" {{if eq .LogLevel "debug"}}selected{{end}}>Debug</option>
                        <option value="info" {{if eq .LogLevel "info"}}selected{{end}}>Info</option>
                        <option value="warn" {{if eq .LogLevel "warn"}}selected{{end}}>Warn</option>
                        <option value="error" {{if eq .LogLevel "error"}}selected{{end}}>Error</option>
                    </select>
                </div>
            </div>

            <!-- Hooks -->
            <div class="card">
                <div class="card-title">
                    🪝 Hooks
                    {{if .HooksHasToken}}<span class="secret-badge set">Token ✓</span>{{else}}<span class="secret-badge not-set">No Token</span>{{end}}
                </div>
                <div class="form-group">
                    <div class="toggle-row">
                        <label class="toggle">
                            <input type="checkbox" id="hooksEnabled" name="hooksEnabled" {{if .HooksEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                        <span class="toggle-label">Enable webhook endpoints</span>
                    </div>
                </div>
                <div class="form-group">
                    <label for="hooksToken">New Token (leave empty to keep current)</label>
                    <input type="text" id="hooksToken" name="hooksToken" placeholder="Enter new token to change">
                    <div class="hint">Used for Authorization: Bearer &lt;token&gt;</div>
                </div>
            </div>

            <div class="actions">
                <button type="submit" class="btn btn-primary" id="save-btn">
                    Save Configuration
                </button>
            </div>
        </form>

        <footer>
            FeelPulse • Configuration Editor
        </footer>
    </div>

    <script>
        const form = document.getElementById('config-form');
        const saveBtn = document.getElementById('save-btn');
        const alertContainer = document.getElementById('alert-container');

        function showAlert(type, message, warnings) {
            let html = '<div class="alert alert-' + type + '">' + message;
            if (warnings && warnings.length > 0) {
                html += '<ul class="warning-list">';
                warnings.forEach(w => {
                    html += '<li>' + w + '</li>';
                });
                html += '</ul>';
            }
            html += '</div>';
            alertContainer.innerHTML = html;
            window.scrollTo({ top: 0, behavior: 'smooth' });
        }

        function clearAlert() {
            alertContainer.innerHTML = '';
        }

        form.addEventListener('submit', async function(e) {
            e.preventDefault();
            clearAlert();

            saveBtn.disabled = true;
            saveBtn.innerHTML = '<span class="spinner"></span>Saving...';

            const data = {
                agentModel: document.getElementById('agentModel').value,
                agentMaxTokens: parseInt(document.getElementById('agentMaxTokens').value) || 4096,
                agentMaxContextTokens: parseInt(document.getElementById('agentMaxContextTokens').value) || 80000,
                agentSystem: document.getElementById('agentSystem').value,
                agentRateLimit: parseInt(document.getElementById('agentRateLimit').value) || 0,
                execEnabled: document.getElementById('execEnabled').checked,
                execAllowedCommands: document.getElementById('execAllowedCommands').value,
                execTimeoutSeconds: parseInt(document.getElementById('execTimeoutSeconds').value) || 30,
                fileEnabled: document.getElementById('fileEnabled').checked,
                browserEnabled: document.getElementById('browserEnabled').checked,
                browserHeadless: document.getElementById('browserHeadless').checked,
                heartbeatEnabled: document.getElementById('heartbeatEnabled').checked,
                heartbeatIntervalMinutes: parseInt(document.getElementById('heartbeatIntervalMinutes').value) || 60,
                logLevel: document.getElementById('logLevel').value,
                hooksEnabled: document.getElementById('hooksEnabled').checked,
                hooksToken: document.getElementById('hooksToken').value
            };

            try {
                const resp = await fetch('/api/config', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + (document.cookie.match(/token=([^;]+)/)?.[1] || '')
                    },
                    body: JSON.stringify(data)
                });

                const result = await resp.json();

                if (result.ok) {
                    showAlert('success', result.message || 'Config saved successfully!', result.warnings);
                    // Clear the token field after successful save
                    document.getElementById('hooksToken').value = '';
                } else {
                    showAlert('error', result.error || 'Failed to save config', result.warnings);
                }
            } catch (err) {
                showAlert('error', 'Network error: ' + err.message);
            } finally {
                saveBtn.disabled = false;
                saveBtn.innerHTML = 'Save Configuration';
            }
        });
    </script>
</body>
</html>`

// generateConfigPageHTML renders the config page HTML
func generateConfigPageHTML(data ConfigPageData) string {
	tmpl, err := template.New("config").Parse(configPageTemplate)
	if err != nil {
		return "<html><body>Error: " + err.Error() + "</body></html>"
	}

	var buf []byte
	writer := &bytesWriter{buf: &buf}
	if err := tmpl.Execute(writer, data); err != nil {
		return "<html><body>Error: " + err.Error() + "</body></html>"
	}

	return string(buf)
}

// parseInt is a helper to parse int with default
func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
