package gateway

import (
	"fmt"
	"html/template"
	"net/http"
	"time"
)

// DashboardData holds data for the dashboard
type DashboardData struct {
	Status         string          `json:"status"`
	Version        string          `json:"version"`
	Uptime         string          `json:"uptime"`
	UptimeSeconds  int64           `json:"uptime_seconds"`
	StartedAt      string          `json:"started_at"`
	ActiveSessions int             `json:"active_sessions"`
	TotalTokens    int             `json:"total_tokens"`
	InputTokens    int             `json:"input_tokens"`
	OutputTokens   int             `json:"output_tokens"`
	TotalRequests  int             `json:"total_requests"`
	Channels       map[string]bool `json:"channels"`
	Agent          string          `json:"agent"`
	RecentActivity []ActivityEntry `json:"recent_activity"`
}

// ActivityEntry represents recent activity
type ActivityEntry struct {
	Timestamp string `json:"timestamp"`
	Channel   string `json:"channel"`
	User      string `json:"user"`
	Preview   string `json:"preview"`
}

// handleDashboard serves the web dashboard
func (gw *Gateway) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !gw.checkAuth(w, r) {
		return
	}

	data := gw.collectDashboardData()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(generateDashboardHTML(data)))
}

// collectDashboardData gathers current system state
func (gw *Gateway) collectDashboardData() DashboardData {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	data := DashboardData{
		Status:   "running",
		Version:  "0.1.0",
		Channels: make(map[string]bool),
	}

	// Calculate uptime
	if !gw.startTime.IsZero() {
		uptime := time.Since(gw.startTime)
		data.UptimeSeconds = int64(uptime.Seconds())
		data.Uptime = formatUptime(data.UptimeSeconds)
		data.StartedAt = gw.startTime.Format(time.RFC3339)
	}

	// Channels status
	data.Channels["telegram"] = gw.telegram != nil

	// Agent info
	if gw.router != nil {
		data.Agent = fmt.Sprintf("%s/%s", gw.cfg.Agent.Provider, gw.cfg.Agent.Model)
	} else {
		data.Agent = "not configured"
	}

	// Session count
	data.ActiveSessions = gw.sessions.Count()

	// Usage stats (aggregate all sessions)
	if gw.usage != nil {
		stats := gw.usage.GetGlobal()
		data.TotalTokens = stats.TotalTokens
		data.InputTokens = stats.InputTokens
		data.OutputTokens = stats.OutputTokens
		data.TotalRequests = stats.RequestCount
	}

	// Recent activity
	data.RecentActivity = gw.getRecentActivity()

	return data
}

// getRecentActivity returns recent session activity
func (gw *Gateway) getRecentActivity() []ActivityEntry {
	sessions := gw.sessions.GetRecent(10)
	activity := make([]ActivityEntry, 0, len(sessions))

	for _, sess := range sessions {
		messages := sess.GetHistory(1)
		if len(messages) == 0 {
			continue
		}

		lastMsg := messages[0]
		activity = append(activity, ActivityEntry{
			Timestamp: lastMsg.Timestamp.Format(time.RFC3339),
			Channel:   sess.Channel(),
			User:      sess.UserID(),
			Preview:   truncatePreview(lastMsg.Text, 50),
		})
	}

	return activity
}

// formatUptime formats seconds as human-readable duration
func formatUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		mins := seconds / 60
		secs := seconds % 60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	if seconds < 86400 {
		hours := seconds / 3600
		mins := (seconds % 3600) / 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	return fmt.Sprintf("%dd %dh", days, hours)
}

// truncatePreview truncates text to maxLen characters
func truncatePreview(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FeelPulse Dashboard</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #eee;
            min-height: 100vh;
            padding: 2rem;
        }
        .container { max-width: 1200px; margin: 0 auto; }
        header {
            display: flex;
            align-items: center;
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .logo { font-size: 2.5rem; }
        h1 { font-size: 2rem; font-weight: 600; }
        .subtitle { color: #888; font-size: 0.9rem; margin-top: 0.25rem; }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 1.5rem;
        }
        .card {
            background: rgba(255,255,255,0.05);
            border-radius: 12px;
            padding: 1.5rem;
            border: 1px solid rgba(255,255,255,0.1);
        }
        .card-title {
            font-size: 0.85rem;
            color: #888;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 0.75rem;
        }
        .card-value {
            font-size: 2rem;
            font-weight: 700;
            color: #fff;
        }
        .card-value.status-running { color: #4ade80; }
        .card-value.status-stopped { color: #f87171; }
        .stat-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
        .stat { }
        .stat-label { color: #888; font-size: 0.8rem; }
        .stat-value { font-size: 1.25rem; font-weight: 600; }
        .channel-list { display: flex; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.5rem; }
        .channel {
            padding: 0.25rem 0.75rem;
            border-radius: 20px;
            font-size: 0.8rem;
            font-weight: 500;
        }
        .channel.active { background: #166534; color: #4ade80; }
        .channel.inactive { background: #374151; color: #9ca3af; }
        .activity-list { }
        .activity-item {
            padding: 0.75rem 0;
            border-bottom: 1px solid rgba(255,255,255,0.1);
        }
        .activity-item:last-child { border-bottom: none; }
        .activity-meta {
            font-size: 0.75rem;
            color: #888;
            margin-bottom: 0.25rem;
        }
        .activity-preview { font-size: 0.9rem; }
        .full-width { grid-column: 1 / -1; }
        footer {
            margin-top: 2rem;
            text-align: center;
            color: #666;
            font-size: 0.8rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <span class="logo">🫀</span>
            <div>
                <h1>FeelPulse Dashboard</h1>
                <p class="subtitle">v{{.Version}} • AI Assistant Platform</p>
            </div>
        </header>

        <div class="grid">
            <div class="card">
                <div class="card-title">Status</div>
                <div class="card-value status-{{.Status}}">{{.Status}}</div>
            </div>

            <div class="card">
                <div class="card-title">Uptime</div>
                <div class="card-value">{{.Uptime}}</div>
            </div>

            <div class="card">
                <div class="card-title">Active Sessions</div>
                <div class="card-value">{{.ActiveSessions}}</div>
            </div>

            <div class="card">
                <div class="card-title">Agent</div>
                <div class="card-value" style="font-size: 1rem;">{{.Agent}}</div>
            </div>

            <div class="card">
                <div class="card-title">Token Usage</div>
                <div class="stat-grid">
                    <div class="stat">
                        <div class="stat-label">Total</div>
                        <div class="stat-value">{{.TotalTokens}}</div>
                    </div>
                    <div class="stat">
                        <div class="stat-label">Requests</div>
                        <div class="stat-value">{{.TotalRequests}}</div>
                    </div>
                    <div class="stat">
                        <div class="stat-label">Input</div>
                        <div class="stat-value">{{.InputTokens}}</div>
                    </div>
                    <div class="stat">
                        <div class="stat-label">Output</div>
                        <div class="stat-value">{{.OutputTokens}}</div>
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-title">Channels</div>
                <div class="channel-list">
                    {{range $name, $active := .Channels}}
                    <span class="channel {{if $active}}active{{else}}inactive{{end}}">
                        {{if $active}}✓{{else}}✗{{end}} {{$name}}
                    </span>
                    {{end}}
                </div>
            </div>

            {{if .RecentActivity}}
            <div class="card full-width">
                <div class="card-title">Recent Activity</div>
                <div class="activity-list">
                    {{range .RecentActivity}}
                    <div class="activity-item">
                        <div class="activity-meta">{{.Channel}} • {{.User}} • {{.Timestamp}}</div>
                        <div class="activity-preview">{{.Preview}}</div>
                    </div>
                    {{end}}
                </div>
            </div>
            {{end}}
        </div>

        <footer>
            FeelPulse • Fast AI Assistant Platform
        </footer>
    </div>
</body>
</html>`

// generateDashboardHTML renders the dashboard HTML
func generateDashboardHTML(data DashboardData) string {
	tmpl, err := template.New("dashboard").Parse(dashboardTemplate)
	if err != nil {
		return fmt.Sprintf("<html><body>Error: %v</body></html>", err)
	}

	var buf []byte
	writer := &bytesWriter{buf: &buf}
	if err := tmpl.Execute(writer, data); err != nil {
		return fmt.Sprintf("<html><body>Error: %v</body></html>", err)
	}

	return string(buf)
}

// bytesWriter implements io.Writer for template execution
type bytesWriter struct {
	buf *[]byte
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
