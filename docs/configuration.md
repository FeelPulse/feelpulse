# Configuration Reference

FeelPulse is configured via `~/.feelpulse/config.yaml`. This document describes all available configuration options.

## Table of Contents

- [Gateway](#gateway)
- [Agent](#agent)
- [Channels](#channels)
  - [Telegram](#telegram)
  - [Discord](#discord)
- [Tools](#tools)
  - [Exec](#exec)
  - [File](#file)
- [Browser](#browser)
- [Workspace](#workspace)
- [Heartbeat](#heartbeat)
- [TTS](#tts)
- [Hooks](#hooks)
- [Metrics](#metrics)
- [Admin](#admin)
- [Log](#log)

---

## Gateway

HTTP server configuration for the gateway.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `gateway.port` | int | `18789` | HTTP port for the gateway server |
| `gateway.bind` | string | `"localhost"` | Bind address (`"0.0.0.0"` for all interfaces) |

```yaml
gateway:
  port: 18789
  bind: localhost
```

---

## Agent

AI provider and model configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `agent.provider` | string | `"anthropic"` | AI provider: `anthropic` or `openai` |
| `agent.model` | string | `"claude-sonnet-4-20250514"` | Model to use |
| `agent.apiKey` | string | `""` | Anthropic API key (`sk-ant-api...`) |
| `agent.authToken` | string | `""` | Claude subscription token (`sk-ant-oat...`) for OAuth auth |
| `agent.maxTokens` | int | `4096` | Maximum tokens in response |
| `agent.maxContextTokens` | int | `80000` | Threshold for context compaction (tokens) |
| `agent.system` | string | `""` | System prompt (overridden by SOUL.md if present) |
| `agent.fallbackModel` | string | `""` | Fallback model on primary failure |
| `agent.fallbackProvider` | string | `""` | Fallback provider (defaults to primary) |
| `agent.rateLimit` | int | `0` | Max messages per minute per user (`0` = disabled) |

### Authentication

You must provide **one of**:
- `apiKey` — Standard Anthropic API key from console.anthropic.com
- `authToken` — Claude subscription setup token (run `claude setup-token`)

```yaml
agent:
  provider: anthropic
  model: claude-sonnet-4-20250514
  apiKey: sk-ant-api03-...
  # OR
  # authToken: sk-ant-oat01-...
  maxTokens: 4096
  maxContextTokens: 80000
  rateLimit: 10
  fallbackModel: claude-3-haiku-20240307
```

### Supported Models

- `claude-sonnet-4-20250514` (default, recommended)
- `claude-opus-4-20250514`
- `claude-3-5-sonnet-20241022`
- `claude-3-opus-20240229`
- `claude-3-sonnet-20240229`
- `claude-3-haiku-20240307`

---

## Channels

### Telegram

Telegram bot configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `channels.telegram.enabled` | bool | `false` | Enable Telegram bot |
| `channels.telegram.token` | string | `""` | Bot token from @BotFather |
| `channels.telegram.allowedUsers` | []string | `[]` | Allowed usernames (empty = all allowed) |

```yaml
channels:
  telegram:
    enabled: true
    token: "123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
    allowedUsers:
      - alice
      - bob
```

**Security Note:** When `allowedUsers` is empty, **anyone** can use your bot. Set this list in production!

### Discord

Discord bot configuration (beta).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `channels.discord.enabled` | bool | `false` | Enable Discord bot |
| `channels.discord.token` | string | `""` | Discord bot token |

```yaml
channels:
  discord:
    enabled: true
    token: "MTIzNDU2Nzg5MDEyMzQ1Njc4OQ...."
```

---

## Tools

### Exec

Shell command execution tool (⚠️ security risk).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tools.exec.enabled` | bool | `false` | Enable exec tool |
| `tools.exec.allowedCommands` | []string | `[]` | Allowed command prefixes (empty = deny all) |
| `tools.exec.timeoutSeconds` | int | `30` | Command timeout in seconds |

```yaml
tools:
  exec:
    enabled: true
    allowedCommands:
      - echo
      - ls
      - cat
      - git status
      - git log
    timeoutSeconds: 30
```

**Security:** Even with allowedCommands, dangerous patterns like `rm -rf /`, `sudo`, and pipe chains are blocked.

### File

Workspace file access tools.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tools.file.enabled` | bool | `true` | Enable file_read/file_write/file_list tools |

```yaml
tools:
  file:
    enabled: true
```

File tools are sandboxed to the workspace directory only. Path traversal (`../`) is blocked.

**Registered tools when enabled:**
- `file_read` — Read file contents (max 100KB)
- `file_write` — Write/create files
- `file_list` — List directory contents

---

## Browser

Browser automation tools using Chrome/Chromium.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `browser.enabled` | bool | `false` | Enable browser tools |
| `browser.headless` | bool | `true` | Run without visible window |
| `browser.timeoutSeconds` | int | `30` | Page load timeout |
| `browser.stealth` | bool | `true` | Use stealth mode to avoid bot detection |

```yaml
browser:
  enabled: true
  headless: true
  stealth: true
  timeoutSeconds: 30
```

**Requirements:** Chrome or Chromium must be installed.

**Registered tools when enabled:**
- `browser_navigate` — Open URL and extract text content
- `browser_screenshot` — Take page/element screenshots
- `browser_click` — Click elements by CSS selector
- `browser_fill` — Fill form fields
- `browser_extract` — Extract data using CSS selectors
- `browser_script` — Execute JavaScript

---

## Workspace

Workspace files (SOUL.md, USER.md, MEMORY.md) and personality profiles.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `workspace.path` | string | `~/.feelpulse/workspace` | Path to workspace directory |
| `workspace.profiles` | map[string]string | `{}` | Named SOUL.md variants |

```yaml
workspace:
  path: ~/.feelpulse/workspace
  profiles:
    friendly: ~/.feelpulse/workspace/profiles/friendly-soul.md
    professional: ~/.feelpulse/workspace/profiles/professional-soul.md
    creative: ~/.feelpulse/workspace/profiles/creative-soul.md
```

**Workspace structure:**
```
~/.feelpulse/workspace/
├── SOUL.md      # AI persona/personality
├── USER.md      # User context
├── MEMORY.md    # Long-term memory
├── memory/      # Daily memory files
└── skills/      # Custom AI tools
```

---

## Heartbeat

Proactive periodic checks.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `heartbeat.enabled` | bool | `false` | Enable heartbeat service |
| `heartbeat.intervalMinutes` | int | `60` | Check interval in minutes |

```yaml
heartbeat:
  enabled: true
  intervalMinutes: 30
```

When enabled, the AI receives a heartbeat prompt periodically and can proactively reach out to users.

---

## TTS

Text-to-speech configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tts.enabled` | bool | `false` | Enable TTS globally |
| `tts.command` | string | `""` | TTS command (auto-detected if empty) |

```yaml
tts:
  enabled: true
  command: ""  # Auto-detects: espeak, say (macOS), festival
```

**Supported TTS engines:**
- `espeak` (Linux)
- `say` (macOS)
- `festival` (Linux)

---

## Hooks

Webhook endpoint configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `hooks.enabled` | bool | `true` | Enable webhook endpoints |
| `hooks.token` | string | `""` | Bearer token for authentication |
| `hooks.path` | string | `"/hooks"` | Base path for webhooks |

```yaml
hooks:
  enabled: true
  token: "secret-webhook-token"
  path: /hooks
```

---

## Metrics

Prometheus metrics endpoint.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics.enabled` | bool | `true` | Enable `/metrics` endpoint |
| `metrics.path` | string | `"/metrics"` | Metrics endpoint path |

```yaml
metrics:
  enabled: true
  path: /metrics
```

**Available metrics:**
- `feelpulse_messages_total{channel}` — Total messages processed
- `feelpulse_tokens_total{type}` — Input/output tokens used
- `feelpulse_active_sessions` — Current active sessions
- `feelpulse_errors_total{type}` — Error counts

---

## Admin

Admin user configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `admin.username` | string | `""` | Admin username (defaults to first allowedUser) |

```yaml
admin:
  username: alice
```

Admin users can access `/admin` commands like `/admin stats`, `/admin sessions`, `/admin reload`.

---

## Log

Logging configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `log.level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |

```yaml
log:
  level: info
```

---

## Complete Example

See [config-example.yaml](config-example.yaml) for a fully commented example configuration.

---

## Environment Variables

FeelPulse does not use environment variables directly, but you can use shell expansion in YAML:

```yaml
agent:
  apiKey: ${ANTHROPIC_API_KEY}  # Won't work - use config file directly
```

For secrets management, consider:
- Using `feelpulse auth` to securely configure API keys
- Mounting secrets via Docker/Kubernetes
- Using a secrets manager to write the config file

---

## Validation

Run `feelpulse status` to validate your configuration:

```bash
$ feelpulse status
✅ Config: ~/.feelpulse/config.yaml
✅ Agent: anthropic/claude-sonnet-4-20250514
✅ Telegram: enabled (token set)
⚠️  Warning: No allowedUsers set - bot is public
✅ Workspace: ~/.feelpulse/workspace
   - SOUL.md: found
   - USER.md: not found
   - MEMORY.md: found
```
