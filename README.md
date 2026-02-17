# 🫀 FeelPulse

Fast AI Assistant Platform in Go. Minimal dependencies, instant startup.

## Features

- **Telegram Bot** - Long polling with Markdown support
- **Anthropic Claude** - Native Messages API client
- **HTTP Gateway** - Health checks and webhooks
- **YAML Config** - Simple configuration

## Quick Start

```bash
# Build
make build

# Initialize config
./build/feelpulse init

# Edit config with your API keys
vim ~/.feelpulse/config.yaml

# Start
./build/feelpulse start
```

## Configuration

After `feelpulse init`, edit `~/.feelpulse/config.yaml`:

```yaml
gateway:
  port: 18789
  bind: localhost

agent:
  model: claude-sonnet-4-20250514
  provider: anthropic
  apiKey: sk-ant-...  # Your Anthropic API key

channels:
  telegram:
    enabled: true
    token: "123456:ABC..."  # Your Telegram bot token

hooks:
  enabled: true
  token: ""  # Optional auth token for webhooks
  path: /hooks
```

### Getting API Keys

1. **Anthropic API Key**: Get from [console.anthropic.com](https://console.anthropic.com)
2. **Telegram Bot Token**: Create a bot via [@BotFather](https://t.me/BotFather)

## Commands

```bash
feelpulse init     # Create default config
feelpulse start    # Start the gateway
feelpulse status   # Check configuration
feelpulse version  # Print version
```

## Makefile Targets

```bash
make build    # Build binary to ./build/
make install  # Install to $GOPATH/bin
make clean    # Remove build artifacts
make test     # Run tests
make run      # Build and run
make dev      # Format, vet, build, run
make check    # Format, vet, test
```

## Architecture

```
feelpulse/
├── cmd/feelpulse/     # CLI entry point
├── internal/
│   ├── agent/         # AI provider clients
│   ├── channel/       # Chat channels (Telegram)
│   ├── config/        # YAML configuration
│   └── gateway/       # HTTP server + routing
└── pkg/types/         # Shared types
```

## API Endpoints

- `GET /health` - Health check with status
- `POST /hooks/*` - Webhook handlers

## Dependencies

- Go 1.21+
- `gopkg.in/yaml.v3` (only external dependency)

## License

MIT
