# FeelPulse

Fast, lightweight AI assistant platform written in Go.

## Why FeelPulse?

- ⚡ **Instant startup** — Go binary, no runtime overhead
- 🧠 **Multi-model** — Claude, GPT, Gemini, local models
- 📱 **Multi-channel** — Telegram, WhatsApp, Discord, WeChat
- 🔌 **Extensible** — Plugin system for channels, tools, hooks
- 🔒 **Secure** — E2E encryption support, token auth
- 🪶 **Lightweight** — Single binary, minimal memory

## Quick Start

```bash
# Install
go install github.com/FeelPulse/feelpulse/cmd/feelpulse@latest

# Initialize
feelpulse init

# Start gateway
feelpulse start

# Check status
feelpulse status
```

## Architecture

```
feelpulse
├── cmd/feelpulse/     # CLI entry point
├── internal/
│   ├── gateway/       # HTTP/WebSocket server
│   ├── config/        # Configuration management
│   ├── channel/       # Messaging channels (Telegram, WhatsApp, etc.)
│   ├── agent/         # AI model routing (Claude, GPT, etc.)
│   └── hook/          # Webhook system
└── pkg/types/         # Shared types
```

## Configuration

```yaml
# ~/.feelpulse/config.yaml
gateway:
  port: 18789
  bind: localhost

agent:
  model: claude-sonnet-4
  provider: anthropic

channels:
  telegram:
    enabled: true
    token: your-bot-token
```

## Development

```bash
# Build
go build -o feelpulse ./cmd/feelpulse

# Run
./feelpulse start

# Test
go test ./...
```

## License

MIT
