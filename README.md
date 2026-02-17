# 🫀 FeelPulse

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-Passing-brightgreen.svg)](https://github.com/FeelPulse/feelpulse/actions)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](Dockerfile)

A fast, minimal AI assistant platform written in Go. FeelPulse provides a Telegram bot interface to Claude AI with support for conversation persistence, workspace files (SOUL.md/USER.md/MEMORY.md), skills/tools, text-to-speech, personality profiles, and more.

**Design Philosophy:** Simple, fast, minimal dependencies. Just Anthropic + Telegram. Built for personal AI assistants.

---

## 🚀 Quick Start

Get running in 5 commands:

```bash
# 1. Clone and build
git clone https://github.com/FeelPulse/feelpulse.git
cd feelpulse && make build

# 2. Initialize config
./build/feelpulse init

# 3. Configure API key (or Claude subscription token)
./build/feelpulse auth

# 4. Add your Telegram bot token to ~/.feelpulse/config.yaml

# 5. Start!
./build/feelpulse start
```

Or with Docker:

```bash
docker run -d -p 18789:18789 -v ~/.feelpulse:/home/feelpulse/.feelpulse feelpulse:latest
```

---

## ✨ Features

### Core
- 🤖 **Claude AI Integration** — Native Anthropic Messages API client (Sonnet 4, Opus 4, etc.)
- 📱 **Telegram Bot** — Long-polling with Markdown support and inline keyboards
- 💾 **Session Persistence** — SQLite-backed conversation history (survives restarts)
- 📂 **Workspace Files** — SOUL.md (persona), USER.md (user context), MEMORY.md (long-term memory)
- 📦 **Context Compaction** — Automatic conversation summarization when context grows large
- 🔄 **Hot Reload** — Config changes apply without restart
- 🛠️ **Tool Calling** — Full agentic loop with tool execution (exec, browser, custom)

### Channels & Interfaces
- 📱 **Telegram Bot** — Rich commands, inline keyboards, file exports
- 🖥️ **TUI** — Interactive terminal chat interface (bubbletea)
- 🌐 **HTTP Gateway** — Health checks, webhooks, OpenAI-compatible API endpoint
- 📊 **Web Dashboard** — Simple status page at `/dashboard`

### Extensions
- 🛠️ **Skills System** — Extensible AI tools via SKILL.md files
- 🔊 **Text-to-Speech** — Auto-detects espeak/say/festival for voice output
- 🎭 **Personality Profiles** — Switch between different SOUL.md variants
- ⏰ **Reminders** — Persistent reminders with relative/absolute time support
- 💓 **Heartbeat** — Proactive periodic checks (optional)
- 🌐 **Browser Automation** — Web scraping and automation tools

### Infrastructure
- ⏱️ **Rate Limiting** — Configurable per-user message rate limits
- 🔒 **User Allowlist** — Restrict bot to specific Telegram usernames
- 🔐 **Dual Auth** — API key or Claude subscription token (sk-ant-oat)
- 🐧 **systemd Service** — Built-in service installation commands
- 📊 **Prometheus Metrics** — `/metrics` endpoint for monitoring
- 🐳 **Docker Ready** — Multi-stage Dockerfile included

---

## 📊 TUI Screenshot

<!-- Add your TUI screenshot here -->
```
┌─────────────────────────────────────────────────┐
│  🫀 FeelPulse TUI                              │
├─────────────────────────────────────────────────┤
│  You: Hello!                                    │
│                                                 │
│  Claude: Hi there! How can I help you today?   │
│                                                 │
│  You: What's the weather like?                  │
│                                                 │
│  Claude: I don't have direct access to weather │
│  data, but I can help you search for it or     │
│  provide general information about weather     │
│  patterns in specific regions.                 │
│                                                 │
├─────────────────────────────────────────────────┤
│  > Type your message...                         │
└─────────────────────────────────────────────────┘
```

---

## 📦 Installation

### From Source

```bash
git clone https://github.com/FeelPulse/feelpulse.git
cd feelpulse
make build
```

### Go Install

```bash
go install github.com/FeelPulse/feelpulse/cmd/feelpulse@latest
```

### Docker

```bash
# Build image
make docker-build

# Or use docker-compose
docker-compose up -d
```

---

## ⚙️ Configuration

After `feelpulse init`, edit `~/.feelpulse/config.yaml`:

```yaml
gateway:
  port: 18789
  bind: localhost

agent:
  model: claude-sonnet-4-20250514
  provider: anthropic
  apiKey: sk-ant-...        # Or use authToken below
  # authToken: sk-ant-oat-... # Use Claude subscription instead of API
  maxTokens: 4096
  maxContextTokens: 80000   # Compaction threshold
  rateLimit: 10             # Messages per minute per user (0 = disabled)
  fallbackModel: claude-3-haiku-20240307  # Optional fallback on error

channels:
  telegram:
    enabled: true
    token: "123456:ABC..."
    allowedUsers:            # Empty = allow all
      - alice
      - bob

hooks:
  enabled: true
  token: ""                 # Optional auth for webhooks
  path: /hooks

workspace:
  path: ~/.feelpulse/workspace
  profiles:                 # Personality profiles
    friendly: ~/.feelpulse/workspace/friendly-soul.md
    professional: ~/.feelpulse/workspace/professional-soul.md

heartbeat:
  enabled: false
  intervalMinutes: 60

tts:
  enabled: false
  command: ""               # Auto-detects: espeak, say (macOS), festival

tools:
  exec:
    enabled: false          # Enable exec tool (security risk!)
    allowedCommands: []     # Whitelist of allowed commands
    timeoutSeconds: 30

browser:
  enabled: false
  headless: true
  stealth: true
  timeoutSeconds: 30

metrics:
  enabled: true
  path: /metrics
```

### Getting API Keys

1. **Anthropic API Key**: Get from [console.anthropic.com](https://console.anthropic.com)
2. **Claude Subscription Token**: Run `claude setup-token` and use `feelpulse auth`
3. **Telegram Bot Token**: Create via [@BotFather](https://t.me/BotFather)

---

## 🖥️ CLI Commands

```bash
feelpulse init           # Create default config
feelpulse auth           # Configure API key or subscription token
feelpulse start          # Start the gateway server
feelpulse status         # Show configuration status

feelpulse workspace init # Create SOUL.md, USER.md, MEMORY.md templates
feelpulse skills list    # List loaded skills

feelpulse tui            # Start interactive terminal chat

feelpulse service install   # Install systemd service
feelpulse service uninstall # Remove systemd service
feelpulse service enable    # Enable on boot
feelpulse service disable   # Disable on boot
feelpulse service status    # Show service status

feelpulse version        # Print version
feelpulse help           # Show help
```

---

## 📱 Telegram Commands

| Command | Description |
|---------|-------------|
| `/new` | Start a new conversation |
| `/history [n]` | Show last n messages (default: 10) |
| `/export` | Export conversation as .txt file |
| `/model [name]` | Show or switch AI model |
| `/models` | List available models |
| `/profile list` | List personality profiles |
| `/profile use <name>` | Switch to a profile |
| `/profile reset` | Reset to default profile |
| `/tts on/off` | Toggle text-to-speech |
| `/skills` | List loaded AI tools |
| `/remind in <time> <msg>` | Set reminder (e.g., `in 30m call mom`) |
| `/remind at <HH:MM> <msg>` | Set reminder at time (e.g., `at 14:00 meeting`) |
| `/reminders` | List active reminders |
| `/cancel <id>` | Cancel a reminder |
| `/usage` | Show token usage stats |
| `/compact` | Force context compaction |
| `/help` | Show all commands |

---

## 🌐 API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check with status |
| `/dashboard` | GET | Simple web dashboard |
| `/metrics` | GET | Prometheus metrics |
| `/v1/chat/completions` | POST | OpenAI-compatible API |
| `/hooks/*` | POST | Webhook handlers |

### OpenAI-Compatible API

FeelPulse exposes an OpenAI-compatible endpoint for integrations:

```bash
curl http://localhost:18789/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

---

## 📂 Workspace Files

Initialize workspace with `feelpulse workspace init`:

```
~/.feelpulse/workspace/
├── SOUL.md     # AI persona/personality (replaces system prompt)
├── USER.md     # User context (name, preferences, etc.)
├── MEMORY.md   # Long-term memory across sessions
└── skills/     # Custom AI tools
    ├── weather/
    │   ├── SKILL.md
    │   └── run.sh
    └── notes/
        └── SKILL.md
```

### SOUL.md Example

```markdown
# Soul

You are a helpful personal assistant named Pulse.

## Personality
- Friendly and warm
- Concise but thorough
- Uses emoji sparingly 

## Guidelines
- Remember context from USER.md
- Update MEMORY.md with important facts
- Be proactive about reminders
```

---

## 🛠️ Skills System

Skills are AI tools defined by `SKILL.md` files:

```
~/.feelpulse/workspace/skills/weather/
├── SKILL.md    # Skill definition
└── run.sh      # Optional executable
```

### SKILL.md Format

```markdown
# weather

Get current weather for a location.

## Parameters
- location (string, required): City or location name
- units (string, optional): Temperature units (celsius/fahrenheit)
```

If `run.sh` exists and is executable, it will be called with parameters as arguments.

---

## ⚡ Performance Benchmarks

Session operations are optimized for low latency:

```
BenchmarkSessionGet              	 7623697	   164.7 ns/op	    23 B/op	   2 allocs/op
BenchmarkSessionGetOrCreate      	  820837	  1442 ns/op	   260 B/op	   4 allocs/op
BenchmarkSessionAddMessage       	 8606056	   130.5 ns/op	   236 B/op	   0 allocs/op
BenchmarkSessionGetAllMessages   	 1145912	   967.3 ns/op	  6144 B/op	   1 allocs/op
BenchmarkContextCompaction       	 1453856	   920.2 ns/op	  1744 B/op	   3 allocs/op
BenchmarkEstimateTokens          	1000000000	 0.37 ns/op	     0 B/op	   0 allocs/op
BenchmarkConcurrentSessionAccess 	  761701	  1432 ns/op	  6400 B/op	   3 allocs/op
BenchmarkSessionCount            	236135662	 5.345 ns/op	     0 B/op	   0 allocs/op
```

Run benchmarks: `make bench`

---

## 📋 Makefile Targets

```bash
# Build & Install
make build           # Build binary to ./build/
make install         # Install to $GOPATH/bin
make clean           # Remove build artifacts

# Run
make start           # Start gateway (foreground)
make start-bg        # Start gateway (background)
make stop            # Stop background gateway
make tui             # Launch terminal chat

# Development
make test            # Run all tests with race detector
make test-short      # Run tests (faster, no race)
make bench           # Run benchmarks
make fmt             # Format code
make vet             # Vet code
make lint            # Run golangci-lint
make check           # Format, vet, and test

# Docker
make docker-build    # Build Docker image
make docker-run      # Run Docker container
make docker-stop     # Stop Docker container
make docker-push     # Push to registry

# Service
make install-service    # Install systemd service
make uninstall-service  # Uninstall systemd service
```

---

## 🏗️ Architecture

```
feelpulse/
├── cmd/feelpulse/     # CLI entry point
├── internal/
│   ├── agent/         # AI providers (Anthropic, OpenAI)
│   ├── browser/       # Browser automation tools
│   ├── channel/       # Chat channels (Telegram, Discord)
│   ├── command/       # Slash command handler
│   ├── config/        # YAML configuration
│   ├── gateway/       # HTTP server, routing, dashboard
│   ├── heartbeat/     # Proactive check service
│   ├── logger/        # Structured logging
│   ├── memory/        # Workspace files manager
│   ├── metrics/       # Prometheus metrics
│   ├── ratelimit/     # Per-user rate limiting
│   ├── scheduler/     # Reminder system
│   ├── session/       # Conversation state, compaction
│   ├── skills/        # Skills/tools loader
│   ├── store/         # SQLite persistence
│   ├── tools/         # Tool registry
│   ├── tts/           # Text-to-speech
│   ├── tui/           # Terminal UI
│   ├── usage/         # Token usage tracking
│   └── watcher/       # Config hot reload
└── pkg/types/         # Shared types
```

---

## 📊 Comparison

| Feature | FeelPulse | OpenClaw | Raw Claude API |
|---------|-----------|----------|----------------|
| Language | Go | TypeScript | — |
| Startup Time | ~10ms | ~500ms | — |
| Memory Usage | ~20MB | ~100MB | — |
| Dependencies | Minimal (4) | Heavy | — |
| Workspace Files | ✅ | ✅ | ❌ |
| Skills System | ✅ | ✅ | ❌ |
| Tool Calling | ✅ | ✅ | ✅ |
| Context Compaction | ✅ | ❌ | ❌ |
| Session Persistence | ✅ SQLite | ✅ | ❌ |
| TTS | ✅ | ✅ | ❌ |
| Hot Reload | ✅ | ❌ | ❌ |
| systemd Service | ✅ | ❌ | ❌ |
| Prometheus Metrics | ✅ | ❌ | ❌ |
| Docker Support | ✅ | ✅ | — |

---

## 📦 Dependencies

- Go 1.23+
- `gopkg.in/yaml.v3` — YAML config
- `github.com/mattn/go-sqlite3` — Session persistence
- `github.com/google/uuid` — UUID generation
- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — TUI styling

---

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Run `make check` before committing
4. Add tests for new features
5. Keep the minimal-dependency philosophy
6. Submit a Pull Request

### Code Style

- Run `make fmt` before committing
- Use meaningful variable names
- Add comments for complex logic
- Keep functions small and focused

---

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

## 🙏 Acknowledgments

- [Anthropic](https://anthropic.com) for Claude
- [Charm](https://charm.sh) for bubbletea TUI framework
- [Telegram](https://telegram.org) for the bot platform

---

Made with 💜 by the FeelPulse team
