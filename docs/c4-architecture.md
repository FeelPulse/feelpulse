# FeelPulse — C4 Architecture

C4 Model: Context → Container → Component → Code

---

## Level 1: System Context

> Who uses FeelPulse and what external systems does it depend on?

```plantuml
@startuml c4-context
!theme plain
allowmixing
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace
skinparam rectangle {
  BackgroundColor #dae8fc
  BorderColor #6c8ebf
}
skinparam actor {
  BackgroundColor #d5e8d4
  BorderColor #82b366
}
skinparam cloud {
  BackgroundColor #fff2cc
  BorderColor #d6b656
}

title System Context — FeelPulse

actor "User" as User #d5e8d4
rectangle "FeelPulse\n\nFast Go-based AI assistant\nplatform. Receives messages\nfrom chat channels, routes\nthem to AI models, returns\nresponses." as FP #dae8fc

cloud "Telegram\napi.telegram.org" as TG #fff2cc
cloud "Anthropic Claude\napi.anthropic.com" as ANT #fff2cc
cloud "OpenAI-compatible\nclients" as OAI #fff2cc
database "SQLite\nsessions.db" as SQL #f8cecc
file "System TTS\nespeak/say" as TTS #f8cecc

User --> TG : sends chat\nmessages
User --> OAI : API requests
TG --> FP : delivers messages\n(long polling)
FP --> TG : sends AI responses
FP --> ANT : calls Claude API\n(streaming SSE)
FP --> SQL : persists sessions\n& reminders
FP --> TTS : speaks responses

note right of FP
  Runs on user's machine\nor VPS as a daemon\n(systemd service)
end note
@enduml
```

**Core relationships:**
- User sends messages via Telegram → FeelPulse receives → calls Claude → replies to Telegram
- FeelPulse runs on user's local machine or VPS, not as a cloud service
- Sessions and reminders persist to SQLite database

---

## Level 2: Container Diagram

> What runnable units make up FeelPulse?

```plantuml
@startuml c4-container
!theme plain
allowmixing
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Container Diagram — FeelPulse

actor "User" as User #d5e8d4

package "FeelPulse Process (single Go binary)" {

  rectangle "CLI\n[Go binary]\n\nEntry point.\nParses subcommands:\nstart / init / auth / tui\nworkspace / skills / service" as CLI #dae8fc

  rectangle "Gateway\n[HTTP Server + Orchestrator]\n\nReceives messages from\nchannels. Orchestrates\nthe full processing\npipeline. Runs on\nlocalhost:18789" as GW #dae8fc

  rectangle "TUI\n[Bubbletea Terminal UI]\n\nInteractive terminal\nchat. Talks directly\nto Agent, bypassing\nTelegram." as TUI #dae8fc

  rectangle "Telegram Channel\n[Long Polling]\n\nPolls Telegram Bot API.\nDelivers messages to\nGateway. Sends replies\nand inline keyboards." as CH #ffe6cc

  rectangle "Agent Router\n[AI Client Layer]\n\nManages Anthropic client.\nHandles API key vs\nOAuth auth. Streaming\nSSE support. Failover." as AR #ffe6cc

  rectangle "Session Store\n[In-Memory + SQLite]\n\nPer-user conversation\nhistory. Keyed by\nchannel:userID.\nAuto-compaction." as SS #d5e8d4

  rectangle "Scheduler\n[Background Goroutine]\n\nFires reminders at\nscheduled times.\nPersists to SQLite." as SC #d5e8d4

  rectangle "Heartbeat Service\n[Background Goroutine]\n\nProactive periodic\nmessages. HEARTBEAT.md\ndriven." as HB #d5e8d4

  rectangle "Config Watcher\n[Polling Goroutine]\n\nWatches config.yaml\nfor changes every 5s.\nTriggers hot reload." as CW #d5e8d4

  rectangle "Rate Limiter\n[In-Memory]\n\nPer-user message\nrate limiting." as RL #d5e8d4

  rectangle "TTS Speaker\n[External Command]\n\nAuto-detects espeak/\nsay/festival. Sanitizes\ntext for speech." as TTS #d5e8d4

  rectangle "Skills Manager\n[File Loader]\n\nLoads SKILL.md files.\nRegisters as tools.\nExecutes run.sh." as SK #d5e8d4
}

database "SQLite\n~/.feelpulse/sessions.db\n\nConversation history\nReminders" as SQL #f8cecc

database "Workspace Files\n~/.feelpulse/workspace/\n\nSOUL.md — persona\nUSER.md — user info\nMEMORY.md — long-term\nskills/ — AI tools" as WS #f8cecc

file "config.yaml\n~/.feelpulse/\n\nGateway, agent,\nchannel settings" as CFG #f8cecc

cloud "Telegram API\napi.telegram.org" as TG #fff2cc
cloud "Anthropic API\napi.anthropic.com" as ANT #fff2cc

User --> CLI : feelpulse start\nfeelpulse tui
CLI --> GW : starts gateway
CLI --> TUI : starts TUI
GW --> CH : registers handler
CH --> TG : long-poll updates\n(getUpdates)
TG --> CH : message events
CH --> GW : handleMessage()
GW --> RL : check rate limit
GW --> SS : load/save history
SS --> SQL : persist
GW --> AR : process messages
AR --> ANT : POST /v1/messages\n(streaming)
TUI --> AR : direct AI calls
SC --> SQL : persist reminders
SC --> CH : send reminder
HB --> CH : send proactive msg
CW --> CFG : fs.Stat() every 5s
CW --> GW : reload config
WS --> GW : loaded on startup
SK --> WS : loads skills/
GW --> TTS : speak response
@enduml
```

**Single-process architecture:** All components run in one Go process, communicating via function calls with zero IPC overhead.

---

## Level 3: Component Diagram — Gateway

> How do Gateway components work together?

```plantuml
@startuml c4-component
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Gateway (internal/gateway)

package "Gateway" {

  component "HTTP Mux\n/health\n/dashboard\n/v1/chat/completions\n/hooks/*" as MUXHTTP
  component "Message\nDispatcher\nhandleMessage()" as DISP
  component "Rate\nLimiter\nAllow(uid)" as RL
  component "Command\nHandler\n/new /model /tts\n/profile /remind" as CMD
  component "Memory\nManager\nSOUL+USER\n+MEMORY+Profile" as MEM
  component "Session\nManager\nGetOrCreate()\nAddMessage()\nPersist()" as SESS
  component "Compactor\ncompactIfNeeded()\n80k token limit" as COMP
  component "Usage\nTracker\nTrack() Report()" as USAGE
  component "TTS\nSpeaker\nSpeak()" as TTS
  component "System Prompt\nBuilder\nAssemblePrompt()" as SPB
}

component "Telegram Bot\nchannel/telegram.go" as TG_BOT
component "Agent Router\nagent/router.go" as AR
component "Scheduler\nscheduler/scheduler.go" as SCHED
component "Heartbeat\nheartbeat/heartbeat.go" as HB
component "Skills Manager\nskills/skills.go" as SKILLS
component "SQLite Store\nstore/sqlite.go" as STORE
component "Config Watcher\nwatcher/watcher.go" as WATCHER

TG_BOT --> DISP : inbound message
DISP --> RL : rate check
RL --> DISP : allowed/denied
DISP --> CMD : IsCommand() check
CMD --> SESS : /new → Clear()
CMD --> SESS : /tts → SetTTS()
CMD --> SESS : /profile → SetProfile()
CMD --> SCHED : /remind → Add()
CMD --> SCHED : /cancel → Cancel()
CMD --> USAGE : /usage → Report()

DISP --> SESS : GetOrCreate(key)
SESS --> STORE : persist
SESS --> COMP : check token count
COMP --> AR : summarize old msgs\n(Claude call)

DISP --> MEM : BuildSystemPrompt()
MEM --> SPB : SOUL.md + Profile\n+ USER.md + MEMORY.md
SPB --> AR : system prompt string
SKILLS --> AR : register tools

DISP --> AR : Process(messages)
AR --> TG_BOT : response text
AR --> USAGE : Track(tokens)

SESS --> TTS : if TTS enabled
TTS --> TTS : Speak(text)

SCHED --> STORE : persist reminders
SCHED --> TG_BOT : send reminders
HB --> TG_BOT : proactive messages
WATCHER --> DISP : reload config
MUXHTTP --> DISP : /v1/chat/completions
@enduml
```

---

## Level 3: Component Diagram — Agent Layer

> How does the Agent handle AI calls?

```plantuml
@startuml c4-agent
!theme plain
allowmixing
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Agent (internal/agent)

component "<<interface>>\nAgent\n+Chat([]Message)\n+Name() string" as IFACE #E8F4FD

component "Router\nagent.go\n\nWraps the active agent.\nHolds per-session\nmodel overrides.\nInjects system prompt." as ROUTER

component "AnthropicClient\nanthropic.go\n\nBuilds HTTP requests.\nHandles API key vs\nOAuth auth modes.\nPosts to Claude API.\nParses SSE stream." as ACLIENT

component "FailoverAgent\nfaiover.go\n\nTries primary agent.\nOn error → tries\nfallback agent.\nLogs degradation." as FAILOVER

component "Summarizer\nsummarizer.go\n\nCalled by Compactor.\nSends old messages\nto Claude with\nsummarize instruction." as SUMM

component "Tool Registry\ntools/tools.go\n\nRegisters built-in tools.\nExec + web_search.\nCalled by agent on\nfunction_call blocks." as TOOLS

component "Skills Manager\nskills/skills.go\n\nLoads SKILL.md files.\nRegisters as tools.\nExecutes run.sh." as SKILLS

cloud "api.anthropic.com\nPOST /v1/messages\n(streaming SSE)" as ANT

ROUTER --> IFACE : implements
ACLIENT --> IFACE : implements
FAILOVER --> IFACE : implements

ROUTER --> FAILOVER : delegates
FAILOVER --> ACLIENT : primary
ACLIENT --> ANT : HTTPS request
ACLIENT --> TOOLS : execute tool calls
SKILLS --> TOOLS : register skill tools
SUMM --> ACLIENT : uses for summarization

note right of ACLIENT
  Auth mode detection:
  sk-ant-oat → OAuth headers
  sk-ant-api → x-api-key
end note
@enduml
```

---

## Level 3: Component Diagram — Session & Persistence

> How do conversation history and persistence work?

```plantuml
@startuml c4-session-memory
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Session, Persistence & Memory

component "Session Store\nsession/session.go" as STORE
component "Store\nmap[key]*Session" as MAP
component "Session\nMessages []Message\nModel string\nTTSEnabled *bool\nProfile string" as SESS_OBJ
STORE --> MAP
STORE --> SESS_OBJ

component "Compactor\nsession/compact.go\n\nEstimate tokens: len/4\nIf > threshold:\n  summarize old msgs\n  replace with Summary msg\n  keep last 10 intact" as COMP

component "SQLite Store\nstore/sqlite.go\n\nPersists sessions.\nPersists reminders.\nSurvives restarts." as SQLITE

component "Memory Manager\nmemory/memory.go\n\nLoads on startup.\nReloads on config change.\nBuilds layered prompt." as MEM

rectangle "SOUL.md\nPersona & identity\nReplaces base system\nprompt if present" as SOUL #FFF2CC
rectangle "USER.md\nUser context:\nname, timezone, prefs\nAppended to prompt" as USER_F #FFF2CC
rectangle "MEMORY.md\nLong-term memories:\ndecisions, events\nAppended to prompt" as MEM_F #FFF2CC
rectangle "Profile\n(from config)\nAlternate persona" as PROF #FFF2CC

database "sessions.db\n\nsessions table\nreminders table" as DB #f8cecc

note as WS_NOTE
  <b>~/.feelpulse/workspace/</b>
end note
WS_NOTE .. SOUL
WS_NOTE .. USER_F
WS_NOTE .. MEM_F

MAP "1" --> "*" SESS_OBJ
SESS_OBJ --> COMP : when len(messages)\nexceeds 80k tokens
STORE --> SQLITE : persists via
SQLITE --> DB : reads/writes

MEM --> SOUL : os.ReadFile()
MEM --> USER_F : os.ReadFile()
MEM --> MEM_F : os.ReadFile()
MEM --> PROF : from config profiles

note right of MEM
  System prompt assembly:
  1. SOUL.md (if exists)
  2. Profile content (if active)
  3. config.agent.system
  4. "## User Context\n" + USER.md
  5. "## Memory\n" + MEMORY.md
end note

note right of COMP
  Before compaction (50 msgs):
  [sys][u1][a1][u2][a2]...[u50][a50]
  
  After compaction:
  [sys][SUMMARY: prev 40 msgs][u41][a41]...[u50][a50]
end note
@enduml
```

---

## Level 3: Component Diagram — Scheduler & TTS

> How do reminders and text-to-speech work?

```plantuml
@startuml c4-scheduler-tts
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Scheduler & TTS

component "Scheduler\nscheduler/scheduler.go" as SCHED
component "Reminders\nmap[id]*Reminder" as REM_MAP
component "Reminder\nID, Channel, UserID\nMessage, FireAt, Created" as REM_OBJ

component "SQLite Store\nReminderPersister" as PERSIST

component "TTS Speaker\ntts/tts.go" as TTS
component "Sanitizer\nremove markdown\nemoji, links" as SANI

note as TTS_NOTE
  Auto-detects:
  - say (macOS)
  - espeak-ng
  - espeak  
  - festival
end note

SCHED --> REM_MAP
REM_MAP --> REM_OBJ
SCHED --> PERSIST : persist on add/cancel/fire
TTS --> SANI : sanitize text
TTS --> TTS_NOTE : uses

note right of SCHED
  /remind in 30m call mom
  /remind at 14:30 meeting
  /cancel abc123
  
  Loads reminders on startup.
  Removes expired.
  Fires due reminders.
end note
@enduml
```

---

## Level 4: Code — Message Processing Sequence

> Full code path from Telegram message to Claude and back

```plantuml
@startuml c4-sequence
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace
skinparam sequenceMessageAlign center

title Code-Level Sequence — "User sends: hi"

participant "Telegram API\napi.telegram.org" as TG
participant "telegram.go\nStartPolling()" as BOT
participant "gateway.go\nhandleMessage()" as GW
participant "ratelimit.go\nLimiter" as RL
participant "command.go\nHandler" as CMD
participant "session.go\nStore" as SESS
participant "store/sqlite.go" as DB
participant "compact.go" as COMP
participant "memory.go\nManager" as MEM
participant "anthropic.go\nAnthropicClient" as ANT_CLIENT
participant "Anthropic API\napi.anthropic.com" as ANT
participant "tts.go\nSpeaker" as TTS
participant "usage.go\nTracker" as USAGE

TG -> BOT : {update_id, message: "hi"}
BOT -> GW : handleMessage(\n  channel="telegram"\n  userID="user123"\n  text="hi"\n)
activate GW

GW -> RL : Allow("user123")
RL -> GW : true

GW -> CMD : IsCommand("hi")
CMD -> GW : false

GW -> SESS : GetOrCreate("telegram:user123")
SESS -> GW : session{messages:[...history...], TTSEnabled:true}

GW -> COMP : CompactIfNeeded(messages, 80000)
note right: EstimateTokens() = sum(len(msg.Text)/4)\nif < 80000: no-op\nif >= 80000: summarize + truncate
COMP -> GW : messages (possibly compacted)

GW -> MEM : BuildSystemPrompt("You are FeelPulse...")
MEM -> GW : "# SOUL\n...\nYou are FeelPulse...\n## User Context\n..."

GW -> ANT_CLIENT : Chat([\n  {role:system, content:prompt},\n  ...history...,\n  {role:user, content:"hi"}\n])
activate ANT_CLIENT

ANT_CLIENT -> ANT_CLIENT : detect auth mode\n(sk-ant-oat → OAuth\nsk-ant-api → API key)

ANT_CLIENT -> ANT : POST /v1/messages\nAuthorization: Bearer sk-ant-oat-...\nanthropic-beta: claude-code-...\n{model, messages, stream:true}
activate ANT

ANT -> ANT_CLIENT : SSE stream:\ndata: {"type":"content_block_delta","delta":{"text":"Hi"}}\ndata: {"type":"content_block_delta","delta":{"text":" there!"}}\n...\ndata: {"type":"message_stop"}
deactivate ANT

ANT_CLIENT -> GW : AgentResponse{\n  text: "Hi there! How can I help?"\n  model: "claude-sonnet-4-20250514"\n  usage: {input:150, output:12}\n}
deactivate ANT_CLIENT

GW -> SESS : AddMessage(user: "hi")
GW -> SESS : AddMessage(bot: "Hi there!...")
GW -> DB : Save("telegram:user123", messages, model)
GW -> USAGE : Track("telegram:user123", 150, 12)

alt TTS enabled for session
  GW -> TTS : Speak("Hi there! How can I help?")
  TTS -> TTS : SanitizeText(text)
  TTS -> TTS : exec("espeak", sanitized)
end

GW -> BOT : return reply message
deactivate GW

BOT -> TG : POST /sendMessage\n{chat_id, text:"Hi there!..."}
TG -> BOT : {ok:true, message_id:123}
@enduml
```

---

## Level 4: Code — Auth Flow

> Code differences between two authentication methods

```plantuml
@startuml c4-auth
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Code-Level — Auth Mode Detection & Header Assembly

start

:Load token from config\n(apiKey or authToken);

if (authToken starts with\n"sk-ant-oat"?) then (yes — OAuth)
  :authMode = AuthModeOAuth;
  
  partition "HTTP Headers (OAuth)" {
    :Authorization: Bearer <token>;
    :anthropic-beta: claude-code-20250219,oauth-2025-04-20;
    :user-agent: claude-cli/1.0.33 (external, cli);
    :x-app: cli;
    :anthropic-dangerous-direct-browser-access: true;
    :anthropic-version: 2023-06-01;
  }
  
  note right
    Mimics Claude Code CLI.
    Anthropic validates against
    your claude.ai subscription.
    No per-token billing.
  end note

else (no — API Key)
  :authMode = AuthModeAPIKey;
  
  partition "HTTP Headers (API Key)" {
    :x-api-key: <apiKey>;
    :anthropic-version: 2023-06-01;
  }
  
  note right
    Standard API auth.
    Billed per input/output token.
    From console.anthropic.com
  end note
endif

:POST https://api.anthropic.com/v1/messages\n{model, messages, max_tokens, stream:true};

:Parse SSE stream response;

stop
@enduml
```

---

## System Design Principles

| Principle | Implementation |
|-----------|----------------|
| **Single-process** | All components in one Go process, zero IPC overhead |
| **SQLite persistence** | Sessions and reminders survive restarts |
| **Streaming responses** | Claude SSE stream → process while generating |
| **Hot reload** | config.yaml changes → watcher detects → auto-reload |
| **Layered prompts** | SOUL → Profile → system → USER → MEMORY stacked |
| **Auto-compaction** | Over 80k tokens → summarize old messages |
| **Subscription auth** | Mimic Claude Code headers → use subscription quota |
| **3ms startup** | Native Go binary, no JVM/V8 startup overhead |
| **Per-session state** | Model, TTS, Profile stored per conversation |
| **Skills extensibility** | SKILL.md + run.sh = custom AI tools |
| **Structured logging** | Log levels (DEBUG/INFO/WARN/ERROR), request IDs |
| **Prometheus metrics** | /metrics endpoint for observability |
| **Exec sandboxing** | Disabled by default, allowlist-only execution |
| **Session branching** | /fork + /sessions + /switch for conversations |
| **Admin commands** | /admin stats/sessions/reload (restricted) |

---

## New Components (Round 4)

### Logger (`internal/logger`)

Structured logging with levels:

```go
// Log levels: DEBUG, INFO, WARN, ERROR
// Format: 2026-02-17 16:00:00 INFO [gateway] Processing message from user123
log := logger.New(&Config{Level: "info", Component: "gateway"})
log.Info("Processing message from %s", username)

// With request context
reqLog := log.WithRequestID("user123")
reqLog.Debug("Session loaded: %d messages", len(messages))
```

### Metrics (`internal/metrics`)

Prometheus-compatible metrics:

```go
metrics.IncrementMessages("telegram")
metrics.AddTokens(inputTokens, outputTokens)
metrics.SetActiveSessions(sessionStore.Count())
metrics.IncrementToolCall("web_search")
```

### Admin Commands

```
/admin stats     — goroutines, memory, uptime
/admin sessions  — all active sessions
/admin reload    — hot-reload config + workspace
```

Restricted to `admin.username` or first `allowedUsers` entry.

### Session Branching

```
/fork [name]     — create copy of conversation
/sessions        — list main + forked sessions
/switch <name>   — change active session
```

In-memory fork with full history copy.
