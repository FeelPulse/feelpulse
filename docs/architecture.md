# FeelPulse Architecture

Fast, lightweight AI assistant platform written in Go. 3ms startup, minimal dependencies.

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
  component [Usage Tracker\ninternal/usage] as UT
  component [Config Watcher\ninternal/watcher] as CW
}

cloud "Channels" {
  component [Telegram Bot\ninternal/channel] as TG
}

cloud "AI Providers" {
  component [Anthropic Claude\napi.anthropic.com] as ANT
}

database "Workspace\n~/.feelpulse/workspace/" as WS {
  file "SOUL.md"
  file "USER.md"
  file "MEMORY.md"
}

file "config.yaml\n~/.feelpulse/" as CFG

User --> TG : message
TG --> GW : handleMessage()
GW --> CH : parse commands
GW --> SS : get/set history
GW --> AR : process(messages)
AR --> ANT : streaming API
AR --> TR : tool calls
MM --> WS : load files
MM --> AR : inject system prompt
SC --> TG : scheduled reminders
CW --> CFG : watch for changes
CW --> GW : reload config
UT --> SS : track token usage

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
participant "CommandHandler" as CH
participant "SessionStore" as SS
participant "AgentRouter" as AR
participant "Telegram" as TG2

TG -> GW : handleMessage(msg)
activate GW

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
  note right: SOUL.md + USER.md\n+ MEMORY.md injected
  AR --> GW : AgentResponse
  deactivate AR
  
  GW -> SS : AddMessage(userMsg)
  GW -> SS : AddMessage(botReply)
  GW -> TG2 : sendMessage(reply)
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
@enduml
```

**Auth modes:**
- `AuthModeAPIKey` — standard `x-api-key` header (sk-ant-api...)
- `AuthModeOAuth` — subscription auth, mimics Claude Code headers (sk-ant-oat...)

### Session Store (`internal/session`)

In-memory conversation history, keyed by `channel:userID`.

```plantuml
@startuml session
!theme plain
skinparam backgroundColor #FAFAFA

class Store {
  -sessions map[string]*Session
  +GetOrCreate(key) *Session
  +Get(key) *Session
  +Delete(key)
}

class Session {
  +Key string
  +Messages []Message
  +Model string
  +CreatedAt time.Time
  +UpdatedAt time.Time
  +AddMessage(msg)
  +Clear()
  +Recent(n) []Message
}

class compact {
  +CompactIfNeeded(messages, maxTokens, summarizer) []Message
  +EstimateTokens(messages) int
}

Store "1" --> "*" Session
Session --> compact : uses
@enduml
```

**Compaction:** When conversation exceeds `maxContextTokens` (default 80k), older messages are summarized via a Claude API call and replaced with a single summary message.

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
}

rectangle "Workspace Files\n(~/.feelpulse/workspace/)" as WS #lightyellow {
  rectangle "SOUL.md\n(persona / AI identity)" as SOUL #fff2cc
  rectangle "USER.md\n(user context)" as USERF #fff2cc
  rectangle "MEMORY.md\n(long-term memory)" as MEMF #fff2cc
}

Manager --> SOUL : reads
Manager --> USERF : reads
Manager --> MEMF : reads
@enduml
```

**System prompt assembly order:**
1. `SOUL.md` content (persona override)
2. Base system prompt from `config.yaml`
3. `USER.md` section
4. `MEMORY.md` section

### Tool Registry (`internal/tools`)

Extensible tool system for function calling.

```plantuml
@startuml tools
!theme plain
skinparam backgroundColor #FAFAFA

class Registry {
  -tools map[string]*Tool
  +Register(tool)
  +Get(name) *Tool
  +List() []*Tool
  +Execute(name, params) string
}

class Tool {
  +Name string
  +Description string
  +Parameters []Parameter
  +Handler func(ctx, params) string
}

rectangle "web_search\n(DuckDuckGo / Brave)" as WS #dae8fc
rectangle "exec\n(Run shell commands)" as EX #dae8fc

Registry "1" --> "*" Tool
Tool --> WS : built-in
Tool --> EX : built-in
@enduml
```

### Scheduler (`internal/scheduler`)

Cron-style reminder system, runs in a background goroutine.

```plantuml
@startuml scheduler
!theme plain
skinparam backgroundColor #FAFAFA

class Scheduler {
  -reminders []*Reminder
  -notify func(chatID, text)
  +Add(chatID, duration, message) string
  +Remove(id)
  +List(chatID) []*Reminder
  +Start(ctx)
}

class Reminder {
  +ID string
  +ChatID string
  +Message string
  +FireAt time.Time
}

Scheduler "1" --> "*" Reminder
Scheduler --> "Telegram\nsendMessage" : fires notification
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
if (slash command?) then (yes)
  :Parse command\n(/new, /model, /usage...);
  :Execute command handler;
  :Return response;
else (no)
  :Load session history;
  if (history > maxTokens?) then (yes)
    :Compact old messages\n(summarize via Claude);
  endif
  :Build messages array\n[system + history + new];
  :Add workspace context\n(SOUL/USER/MEMORY);
endif

|Agent|
:Send to Anthropic API\n(streaming SSE);
:Collect response chunks;
:Track token usage;
:Return AgentResponse;

|Gateway|
:Save messages to session;
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
  system: "You are a helpful AI assistant."

workspace:
  path: ~/.feelpulse/workspace  # SOUL.md, USER.md, MEMORY.md

channels:
  telegram:
    enabled: true
    token: ""

hooks:
  enabled: true
  token: ""
  path: /hooks
```

---

## Directory Structure

```
feelpulse/
├── cmd/feelpulse/
│   └── main.go              # CLI entry point (start/init/auth/tui/status)
├── internal/
│   ├── agent/
│   │   ├── agent.go         # Router + Agent interface
│   │   ├── anthropic.go     # Anthropic client (API key + OAuth)
│   │   ├── failover.go      # Automatic model fallback
│   │   └── summarizer.go    # Conversation compaction helper
│   ├── channel/
│   │   └── telegram.go      # Telegram long-polling bot
│   ├── command/
│   │   └── command.go       # Slash command handler
│   ├── config/
│   │   └── config.go        # YAML config load/save
│   ├── gateway/
│   │   └── gateway.go       # Central message orchestrator
│   ├── hook/
│   │   └── hook.go          # Webhook HTTP handlers
│   ├── memory/
│   │   └── memory.go        # Workspace file loader
│   ├── scheduler/
│   │   └── scheduler.go     # Cron reminders
│   ├── session/
│   │   ├── session.go       # In-memory conversation store
│   │   └── compact.go       # Context compaction
│   ├── tools/
│   │   ├── tools.go         # Tool registry
│   │   └── builtins.go      # exec, web_search
│   ├── usage/
│   │   └── usage.go         # Token usage tracking
│   └── watcher/
│       └── watcher.go       # Config file hot-reload
├── pkg/types/
│   └── message.go           # Shared types (Message, AgentResponse)
├── docs/
│   └── architecture.md      # This file
├── TODO.md
├── Makefile
├── go.mod
└── README.md
```

---

## Available Commands

| Command | Description |
|---------|-------------|
| `make start` | Build and start (foreground) |
| `make start-bg` | Build and start (background) |
| `make stop` | Stop background process |
| `make restart` | Restart background process |
| `make logs` | Tail live logs |
| `make tui` | Launch terminal chat UI |
| `make auth` | Configure API key or setup-token |
| `make test` | Run all tests |
| `make dev` | Format + vet + build + start |

### Slash Commands (in Telegram)

| Command | Description |
|---------|-------------|
| `/new` | Start a new conversation |
| `/history [n]` | Show last n messages |
| `/model [name]` | Show or switch AI model |
| `/usage` | Token usage stats |
| `/remind in <duration> <msg>` | Set a reminder |
| `/reminders` | List active reminders |
| `/help` | Show all commands |
