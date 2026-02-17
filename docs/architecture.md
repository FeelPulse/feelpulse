# FeelPulse Architecture

Fast, lightweight AI assistant platform written in Go. 3ms startup, minimal dependencies.

**Stats:** ~25 packages, ~8000 lines of Go code

---

## Overview

```plantuml
@startuml overview
!theme plain
skinparam backgroundColor #FAFAFA
skinparam defaultFontName monospace

actor User
package "FeelPulse" {
  component [CLI\ncmd/feelpulse] as CLI
  component [Gateway\ninternal/gateway] as GW
  component [Agent Router\ninternal/agent] as AR
  component [Session Store\ninternal/session] as SS
  component [Memory Manager\ninternal/memory] as MM
  component [Command Handler\ninternal/command] as CH
  component [Scheduler\ninternal/scheduler] as SC
  component [Tool Registry\ninternal/tools] as TR
  component [Skills Manager\ninternal/skills] as SK
  component [Usage Tracker\ninternal/usage] as UT
  component [Config Watcher\ninternal/watcher] as CW
  component [Heartbeat Service\ninternal/heartbeat] as HB
  component [Rate Limiter\ninternal/ratelimit] as RL
  component [TTS Speaker\ninternal/tts] as TTS
  component [SQLite Store\ninternal/store] as DB
  component [Logger\ninternal/logger] as LOG
  component [Metrics\ninternal/metrics] as MET
}

cloud "Channels" {
  component [Telegram Bot\ninternal/channel] as TG
  component [Web Dashboard\n/dashboard] as DASH
  component [OpenAI API\n/v1/chat/completions] as OAI
  component [Metrics API\n/metrics] as PROM
}

cloud "AI Providers" {
  component [Anthropic Claude\napi.anthropic.com] as ANT
}

database "SQLite\n~/.feelpulse/sessions.db" as SQL

database "Workspace\n~/.feelpulse/workspace/" as WS {
  file "SOUL.md"
  file "USER.md"
  file "MEMORY.md"
  folder "skills/"
}

file "config.yaml\n~/.feelpulse/" as CFG

User --> TG : message
User --> OAI : API request
TG --> GW : handleMessage()
OAI --> GW : handleOpenAI()
GW --> RL : rate check
GW --> CH : parse commands
GW --> SS : get/set history
SS --> DB : persist
GW --> AR : process(messages)
AR --> ANT : streaming API
AR --> TR : tool calls
SK --> TR : register tools
MM --> WS : load files
MM --> AR : inject system prompt
SC --> DB : persist reminders
SC --> TG : scheduled reminders
HB --> TG : proactive messages
CW --> CFG : watch for changes
CW --> GW : reload config
UT --> SS : track token usage
GW --> TTS : speak response
DASH --> GW : status

CLI --> GW : start
CLI --> CFG : init/auth

@enduml
```

---

## Component Details

### Gateway (`internal/gateway`)

The central orchestrator. Receives messages from channels, routes them through the pipeline, and sends responses back.

```plantuml
@startuml gateway
!theme plain
skinparam backgroundColor #FAFAFA

participant "Telegram" as TG
participant "Gateway" as GW
participant "RateLimiter" as RL
participant "CommandHandler" as CH
participant "SessionStore" as SS
participant "AgentRouter" as AR
participant "TTS" as TTS
participant "Telegram" as TG2

TG -> GW : handleMessage(msg)
activate GW

GW -> RL : Allow(userID)
alt rate limited
  GW -> TG2 : sendMessage("Rate limited")
else allowed
  GW -> CH : IsCommand(msg.Text)
  alt is slash command
    CH -> GW : handleCommand(msg) → reply
    GW -> TG2 : sendMessage(reply)
  else regular message
    GW -> SS : GetOrCreate(channelID+userID)
    SS -> GW : session (with history)
    
    GW -> GW : compactIfNeeded(history)
    GW -> AR : Process(messages)
    activate AR
    AR -> AR : buildSystemPrompt()
    note right: SOUL.md + USER.md\n+ MEMORY.md + Profile
    AR --> GW : AgentResponse
    deactivate AR
    
    GW -> SS : AddMessage(userMsg)
    GW -> SS : AddMessage(botReply)
    GW -> SS : Persist()
    
    alt TTS enabled
      GW -> TTS : Speak(reply.Text)
    end
    
    GW -> TG2 : sendMessage(reply)
  end
end

deactivate GW
@enduml
```

### Agent Router (`internal/agent`)

Manages AI provider clients, handles auth mode detection, streaming, and failover.

```plantuml
@startuml agent
!theme plain
skinparam backgroundColor #FAFAFA

package "internal/agent" {
  class Router {
    +Process(messages) AgentResponse
    +ProcessWithHistory([]Message) AgentResponse
    +SetSystemPromptBuilder(func)
    +Name() string
  }

  class AnthropicClient {
    -apiKey string
    -authToken string
    -authMode AuthMode
    -model string
    +Chat(messages) AgentResponse
    +ChatStream(messages, callback) AgentResponse
    +AuthModeName() string
  }

  class FailoverAgent {
    -primary Agent
    -fallback Agent
    +Chat(messages) AgentResponse
  }

  class Summarizer {
    +SummarizeConversation(messages) string
  }

  enum AuthMode {
    AuthModeAPIKey
    AuthModeOAuth
  }

  interface Agent {
    +Chat(messages) AgentResponse
    +Name() string
  }
}

Router --> Agent : delegates to
AnthropicClient ..|> Agent
FailoverAgent ..|> Agent
FailoverAgent --> Agent : primary
FailoverAgent --> Agent : fallback
AnthropicClient --> AuthMode
Summarizer --> AnthropicClient
@enduml
```

**Auth modes:**
- `AuthModeAPIKey` — standard `x-api-key` header (sk-ant-api...)
- `AuthModeOAuth` — subscription auth, mimics Claude Code headers (sk-ant-oat...)

### Session Store (`internal/session`)

In-memory conversation history with SQLite persistence, keyed by `channel:userID`.

```plantuml
@startuml session
!theme plain
skinparam backgroundColor #FAFAFA

class Store {
  -sessions map[string]*Session
  -persister Persister
  +GetOrCreate(key) *Session
  +Get(key) *Session
  +Delete(key)
  +SetPersister(p)
  +AddMessageAndPersist(ch, uid, msg)
}

class Session {
  +Key string
  +Messages []Message
  +Model string
  +TTSEnabled *bool
  +Profile string
  +CreatedAt time.Time
  +UpdatedAt time.Time
  +AddMessage(msg)
  +Clear()
  +SetTTS(enabled)
  +SetProfile(name)
  +SetModel(model)
}

class Compactor {
  -summarizer Summarizer
  -maxTokens int
  +CompactIfNeeded(messages) []Message
  +EstimateTokens(messages) int
}

interface Persister {
  +Save(key, messages, model) error
  +Load(key) (messages, model, error)
  +Delete(key) error
  +ListKeys() ([]string, error)
}

Store "1" --> "*" Session
Store --> Persister : persists via
Session --> Compactor : uses
@enduml
```

**Compaction:** When conversation exceeds `maxContextTokens` (default 80k), older messages are summarized via a Claude API call and replaced with a single summary message.

### SQLite Store (`internal/store`)

Persists sessions and reminders to SQLite database.

```plantuml
@startuml store
!theme plain
skinparam backgroundColor #FAFAFA

class SQLiteStore {
  -db *sql.DB
  +Save(key, messages, model) error
  +Load(key) (messages, model, error)
  +Delete(key) error
  +ListKeys() []string
  +SaveReminder(r) error
  +DeleteReminder(id) error
  +LoadReminders() []*ReminderData
  +CleanExpiredReminders() int64
}

database "sessions.db" as DB

note bottom of DB
  sessions: key, messages, model, updated_at
  reminders: id, channel, user_id, message, fire_at, created
end note

SQLiteStore --> DB : reads/writes
@enduml
```

### Memory Manager (`internal/memory`)

Loads workspace files and injects them into the system prompt.

```plantuml
@startuml memory
!theme plain
skinparam backgroundColor #FAFAFA

class Manager {
  -path string
  -soul string
  -user string
  -memory string
  +Load() error
  +BuildSystemPrompt(base string) string
  +Soul() string
  +User() string
  +Memory() string
}

note as WS
  <b>~/.feelpulse/workspace/</b>
  SOUL.md — persona / AI identity
  USER.md — user context
  MEMORY.md — long-term memory
end note

Manager --> WS : reads files
@enduml
```

**System prompt assembly order:**
1. `SOUL.md` content (persona override)
2. Base system prompt from `config.yaml`
3. `USER.md` section
4. `MEMORY.md` section
5. Profile content (if `/profile use <name>`)

### Skills System (`internal/skills`)

Extensible tool system for function calling via SKILL.md files.

```plantuml
@startuml skills
!theme plain
allowmixing
skinparam backgroundColor #FAFAFA

class Manager {
  -dir string
  -skills []*Skill
  -registry *Registry
  +Reload() error
  +ListSkills() []*Skill
  +GetSkill(name) *Skill
  +Registry() *Registry
}

class Loader {
  -dir string
  +Load() []*Skill
}

class Skill {
  +Name string
  +Description string
  +Parameters []Param
  +Executable string
  +Dir string
  +ToTool() *Tool
}

note as SKILL
  <b>~/.feelpulse/workspace/skills/</b>
  weather/
    SKILL.md
    run.sh
  notes/
    SKILL.md
end note

Manager --> Loader : loads via
Loader --> SKILL : parses
Manager --> Skill : manages
Skill --> Tool : converts to
@enduml
```

### Scheduler (`internal/scheduler`)

Persistent reminder system with SQLite backing.

```plantuml
@startuml scheduler
!theme plain
skinparam backgroundColor #FAFAFA

class Scheduler {
  -reminders map[string]*Reminder
  -persister ReminderPersister
  -handler ReminderHandler
  +AddReminder(ch, uid, duration, msg) (id, error)
  +Cancel(id) bool
  +List(ch, uid) []*Reminder
  +SetPersister(p) error
  +Start()
  +Stop()
}

class Reminder {
  +ID string
  +Channel string
  +UserID string
  +Message string
  +FireAt time.Time
  +Created time.Time
  +String() string
}

interface ReminderPersister {
  +SaveReminder(r) error
  +DeleteReminder(id) error
  +LoadReminders() []*ReminderData
}

Scheduler "1" --> "*" Reminder
Scheduler --> ReminderPersister : persists via
Scheduler --> "Telegram\nsendMessage" : fires notification
@enduml
```

**Features:**
- `/remind in 30m call mom` — relative time
- `/remind at 14:30 meeting` — absolute time
- `/cancel <id>` — cancel by ID prefix
- Persists to SQLite (survives restarts)

### TTS Speaker (`internal/tts`)

Text-to-speech output with auto-detection.

```plantuml
@startuml tts
!theme plain
skinparam backgroundColor #FAFAFA

class Speaker {
  +Command string
  +Speak(text) error
  +Available() bool
  +BuildCommand(text) (cmd, args, stdin)
  +SanitizeText(text) string
}

note as CMDS
  Auto-detects:
  - say (macOS)
  - espeak-ng
  - espeak
  - festival
end note

Speaker --> CMDS : uses first available
@enduml
```

**Text sanitization:**
- Removes emoji, markdown, links
- Handles code blocks
- Collapses whitespace

### Heartbeat Service (`internal/heartbeat`)

Proactive periodic check system.

```plantuml
@startuml heartbeat
!theme plain
skinparam backgroundColor #FAFAFA

class Service {
  -config *Config
  -workspacePath string
  -users map[string]map[int64]bool
  -callback func(ch, uid, msg)
  +RegisterUser(ch, uid)
  +SetCallback(fn)
  +Start()
  +Stop()
}

note as HB
  Reads HEARTBEAT.md from workspace
  Triggers periodic messages
  Registers active users from Telegram
end note

Service --> HB
@enduml
```

### Rate Limiter (`internal/ratelimit`)

Per-user message rate limiting.

```plantuml
@startuml ratelimit
!theme plain
skinparam backgroundColor #FAFAFA

class Limiter {
  -rate int
  -users map[string]*userState
  +Allow(userID) bool
}

note as RL
  rate = messages per minute
  0 = disabled
  Sliding window algorithm
end note

Limiter --> RL
@enduml
```

---

## Data Flow

### Message Processing Pipeline

```plantuml
@startuml dataflow
!theme plain
skinparam backgroundColor #FAFAFA
skinparam ArrowColor #555

|Telegram|
start
:User sends message;

|Gateway|
:Receive update;
:Check rate limit;
if (rate limited?) then (yes)
  :Return "Rate limited";
  stop
endif

if (slash command?) then (yes)
  :Parse command\n(/new, /tts, /profile...);
  :Execute command handler;
  :Return response;
else (no)
  :Load session history;
  :Load session preferences\n(model, TTS, profile);
  if (history > maxTokens?) then (yes)
    :Compact old messages\n(summarize via Claude);
  endif
  :Build messages array\n[system + history + new];
  :Add workspace context\n(SOUL/USER/MEMORY/Profile);
endif

|Agent|
:Send to Anthropic API\n(streaming SSE);
:Collect response chunks;
:Track token usage;
:Return AgentResponse;

|Gateway|
:Save messages to session;
:Persist to SQLite;

if (TTS enabled for session?) then (yes)
  :Sanitize text;
  :Call TTS command;
endif

:Send reply to Telegram;

|Telegram|
:User receives response;
stop
@enduml
```

### Authentication Flow

```plantuml
@startuml auth
!theme plain
skinparam backgroundColor #FAFAFA

start
:Read token from config;
if (starts with "sk-ant-oat"?) then (OAuth)
  :Set Authorization: Bearer <token>;
  :Set anthropic-beta: claude-code-20250219,oauth-2025-04-20;
  :Set user-agent: claude-cli/<version>;
  :Set x-app: cli;
  note right: Mimics Claude Code CLI\nUses subscription quota
else (API Key)
  :Set x-api-key: <key>;
  note right: Pay-per-token\nStandard API
endif
:Call api.anthropic.com/v1/messages;
stop
@enduml
```

---

## Configuration

```yaml
# ~/.feelpulse/config.yaml
gateway:
  port: 18789
  bind: localhost

agent:
  provider: anthropic
  model: claude-sonnet-4-20250514
  apiKey: ""          # sk-ant-api-... (pay-per-token)
  authToken: ""       # sk-ant-oat-... (Claude subscription)
  maxTokens: 4096
  maxContextTokens: 80000
  rateLimit: 10       # messages per minute per user (0 = disabled)
  fallbackModel: claude-3-haiku-20240307
  system: "You are a helpful AI assistant."

workspace:
  path: ~/.feelpulse/workspace  # SOUL.md, USER.md, MEMORY.md
  profiles:
    friendly: ~/.feelpulse/workspace/friendly-soul.md
    professional: ~/.feelpulse/workspace/professional-soul.md

channels:
  telegram:
    enabled: true
    token: ""
    allowedUsers:       # empty = allow all
      - alice
      - bob

hooks:
  enabled: true
  token: ""
  path: /hooks

heartbeat:
  enabled: false
  intervalMinutes: 60

tts:
  enabled: false
  command: ""           # auto-detects: espeak, say, festival

browser:
  enabled: false        # Requires Chrome/Chromium
  headless: true
  timeoutSeconds: 30
  stealth: true

# Security settings for exec tool
tools:
  exec:
    enabled: false      # Disabled by default for security
    allowedCommands: [] # e.g. ["echo", "ls", "cat", "git"]
    timeoutSeconds: 30

# Logging
log:
  level: info           # debug, info, warn, error

# Admin commands
admin:
  username: ""          # Defaults to first allowedUser

# Metrics endpoint
metrics:
  enabled: true
  path: /metrics
```

---

## Directory Structure

```
feelpulse/
├── cmd/feelpulse/
│   ├── main.go              # CLI entry point
│   └── service.go           # systemd service management
├── internal/
│   ├── agent/
│   │   ├── agent.go         # Router + Agent interface
│   │   ├── anthropic.go     # Anthropic client (API key + OAuth)
│   │   ├── failover.go      # Automatic model fallback
│   │   └── summarizer.go    # Conversation compaction helper
│   ├── browser/
│   │   └── browser.go       # Browser automation (Chromedp)
│   ├── channel/
│   │   ├── telegram.go      # Telegram long-polling bot
│   │   └── keyboard.go      # Inline keyboards, bot commands
│   ├── command/
│   │   └── command.go       # Slash command handler
│   ├── config/
│   │   └── config.go        # YAML config load/save
│   ├── gateway/
│   │   ├── gateway.go       # Central orchestrator
│   │   ├── dashboard.go     # Web dashboard
│   │   └── openai.go        # OpenAI-compatible API
│   ├── heartbeat/
│   │   └── heartbeat.go     # Proactive check service
│   ├── logger/
│   │   └── logger.go        # Structured logging with levels
│   ├── memory/
│   │   └── memory.go        # Workspace file loader
│   ├── metrics/
│   │   └── metrics.go       # Prometheus-compatible metrics
│   ├── ratelimit/
│   │   └── limiter.go       # Per-user rate limiting
│   ├── scheduler/
│   │   └── scheduler.go     # Persistent reminders
│   ├── session/
│   │   ├── session.go       # Conversation store + branching
│   │   └── compact.go       # Context compaction
│   ├── skills/
│   │   └── skills.go        # SKILL.md loader + executor
│   ├── store/
│   │   └── sqlite.go        # SQLite persistence
│   ├── tools/
│   │   ├── tools.go         # Tool registry
│   │   └── builtins.go      # Built-in tools (exec, web_search)
│   ├── tts/
│   │   └── tts.go           # Text-to-speech
│   ├── tui/
│   │   └── model.go         # Terminal UI (bubbletea)
│   ├── usage/
│   │   └── usage.go         # Token usage tracking
│   └── watcher/
│       └── watcher.go       # Config file hot-reload
├── pkg/types/
│   └── message.go           # Shared types
├── docs/
│   ├── architecture.md      # This file
│   └── c4-architecture.md   # C4 model diagrams
├── TODO.md
├── Makefile
├── go.mod
└── README.md
```

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check with status |
| `/dashboard` | GET | Web status dashboard |
| `/v1/chat/completions` | POST | OpenAI-compatible API |
| `/hooks/*` | POST | Webhook handlers |
| `/metrics` | GET | Prometheus-compatible metrics |

---

## Telegram Commands

| Command | Description |
|---------|-------------|
| `/new` | Start a new conversation |
| `/history [n]` | Show last n messages |
| `/export` | Export conversation as .txt file |
| `/compact` | Manually compress conversation history |
| `/model [name]` | Show or switch AI model |
| `/models` | List available models |
| `/profile list` | List personality profiles |
| `/profile use <name>` | Switch to a profile |
| `/fork [name]` | Create conversation fork |
| `/sessions` | List your sessions |
| `/switch <name>` | Switch to session |
| `/tts on/off` | Toggle text-to-speech |
| `/skills` | List loaded AI tools |
| `/remind in <time> <msg>` | Set reminder (relative) |
| `/remind at <HH:MM> <msg>` | Set reminder (absolute) |
| `/reminders` | List active reminders |
| `/cancel <id>` | Cancel a reminder |
| `/usage` | Token usage stats |
| `/admin stats` | System statistics (admin only) |
| `/admin sessions` | All sessions (admin only) |
| `/admin reload` | Reload config (admin only) |
| `/help` | Show all commands |

---

## Metrics

Prometheus-compatible metrics at `GET /metrics`:

```
# HELP feelpulse_messages_total Total messages processed
# TYPE feelpulse_messages_total counter
feelpulse_messages_total{channel="telegram"} 42

# HELP feelpulse_tokens_total Total tokens used
feelpulse_tokens_total{type="input"} 12345
feelpulse_tokens_total{type="output"} 6789

# HELP feelpulse_active_sessions Current active sessions
feelpulse_active_sessions 3

# HELP feelpulse_tool_calls_total Tool calls by tool name
feelpulse_tool_calls_total{tool="web_search"} 10

# HELP feelpulse_tool_errors_total Tool errors by tool name
feelpulse_tool_errors_total{tool="exec"} 1
```

---

## Security

### Exec Tool Security

The exec tool (shell command execution) is **disabled by default** for security. To enable:

```yaml
tools:
  exec:
    enabled: true
    allowedCommands:    # Only these commands can run
      - echo
      - ls
      - cat
      - git
    timeoutSeconds: 30  # Command timeout
```

**Blocked patterns** (even if command is in allowlist):
- `rm -rf /`, `rm -rf ~`, `rm -rf $HOME`
- `sudo`, `su -`
- Path traversal (`../`)
- Piping to shell (`curl ... | sh`)
- Writing to `/etc/`, `/dev/`
- System commands (`reboot`, `shutdown`)

### Admin Commands

Admin commands restricted to configured admin user:

```yaml
admin:
  username: "alice"    # Defaults to first allowedUser
```
