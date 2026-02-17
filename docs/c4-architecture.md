# FeelPulse — C4 Architecture

C4 Model: Context → Container → Component → Code

---

## Level 1: System Context

> 谁在使用 FeelPulse，它依赖哪些外部系统？

```plantuml
@startuml c4-context
!theme plain
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

actor "User\n(Leon)" as User #d5e8d4
rectangle "FeelPulse\n\nFast Go-based AI assistant\nplatform. Receives messages\nfrom chat channels, routes\nthem to AI models, returns\nresponses." as FP #dae8fc

cloud "Telegram\napi.telegram.org" as TG #fff2cc
cloud "Anthropic Claude\napi.anthropic.com" as ANT #fff2cc
cloud "Web Search\n(Brave/DuckDuckGo)" as WEB #fff2cc

User --> TG : sends chat\nmessages
TG --> FP : delivers messages\n(long polling)
FP --> TG : sends AI responses
FP --> ANT : calls Claude API\n(streaming SSE)
FP --> WEB : web_search tool\n(HTTP REST)

note right of FP
  Runs on user's machine\nor VPS as a daemon\n(systemd service)
end note
@enduml
```

**核心关系：**
- User 通过 Telegram 发消息 → FeelPulse 接收 → 调用 Claude → 回复给 Telegram
- FeelPulse 运行在用户本地或 VPS，不是云服务

---

## Level 2: Container Diagram

> FeelPulse 内部由哪些可运行的单元组成？

```plantuml
@startuml c4-container
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Container Diagram — FeelPulse

actor "User" as User #d5e8d4

package "FeelPulse Process (single Go binary)" {

  rectangle "CLI\n[Go binary]\n\nEntry point.\nParses subcommands:\nstart / init / auth / tui / status" as CLI #dae8fc

  rectangle "Gateway\n[HTTP Server + Orchestrator]\n\nReceives messages from\nchannels. Orchestrates\nthe full processing\npipeline. Runs on\nlocalhost:18789" as GW #dae8fc

  rectangle "TUI\n[Bubbletea Terminal UI]\n\nInteractive terminal\nchat. Talks directly\nto Agent, bypassing\nTelegram." as TUI #dae8fc

  rectangle "Telegram Channel\n[Long Polling]\n\nPolls Telegram Bot API\nevery second. Delivers\nmessages to Gateway.\nSends replies back." as CH #ffe6cc

  rectangle "Agent Router\n[AI Client Layer]\n\nManages Anthropic client.\nHandles API key vs\nOAuth (setup-token) auth.\nStreaming SSE support." as AR #ffe6cc

  rectangle "Session Store\n[In-Memory Map]\n\nPer-user conversation\nhistory. Keyed by\nchannel:userID.\nAuto-compaction." as SS #d5e8d4

  rectangle "Scheduler\n[Background Goroutine]\n\nFires reminders at\nscheduled times.\nTicks every second." as SC #d5e8d4

  rectangle "Config Watcher\n[Polling Goroutine]\n\nWatches config.yaml\nfor changes every 5s.\nTriggers hot reload." as CW #d5e8d4
}

database "Workspace Files\n~/.feelpulse/workspace/\n\nSOUL.md — persona\nUSER.md — user info\nMEMORY.md — long-term" as WS #f8cecc

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
GW --> SS : load/save history
GW --> AR : process messages
AR --> ANT : POST /v1/messages\n(streaming)
TUI --> AR : direct AI calls
SC --> CH : send reminder
CW --> CFG : fs.Stat() every 5s
CW --> GW : reload config
WS --> GW : loaded on startup\n+ on config reload
@enduml
```

**单进程架构：** 所有组件在同一个 Go 进程中运行，通过函数调用通信，无 IPC 开销。

---

## Level 3: Component Diagram

> Gateway 内部各组件如何协作？

```plantuml
@startuml c4-component
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Gateway (internal/gateway)

rectangle "Gateway" {

  component "HTTP Mux\n/health\n/hooks/*" as MUXHTTP
  component "Message\nDispatcher\nhandleMessage()" as DISP
  component "Command\nHandler\n/new /model\n/usage /help" as CMD
  component "Memory\nManager\nSOUL+USER\n+MEMORY" as MEM
  component "Session\nManager\nGetOrCreate()\nAddMessage()" as SESS
  component "Compactor\ncompactIfNeeded()\n80k token limit" as COMP
  component "Usage\nTracker\nTrack() Report()" as USAGE
  component "System Prompt\nBuilder\nAssemblePrompt()" as SPB
}

component "Telegram Bot\nchannel/telegram.go" as TG_BOT
component "Agent Router\nagent/router.go" as AR
component "Scheduler\nscheduler/scheduler.go" as SCHED
component "Hook Handler\nhook/hook.go" as HOOK
component "Config Watcher\nwatcher/watcher.go" as WATCHER

TG_BOT --> DISP : inbound message
DISP --> CMD : IsCommand() check
CMD --> SESS : /new → Clear()
CMD --> USAGE : /usage → Report()
CMD --> AR : /model → set override

DISP --> SESS : GetOrCreate(key)
SESS --> COMP : check token count
COMP --> AR : summarize old msgs\n(Claude call)

DISP --> MEM : BuildSystemPrompt()
MEM --> SPB : SOUL.md + base\n+ USER.md + MEMORY.md
SPB --> AR : system prompt string

DISP --> AR : Process(messages)
AR --> TG_BOT : response text
AR --> USAGE : Track(tokens)

SCHED --> TG_BOT : send reminders
HOOK --> DISP : webhook messages
WATCHER --> DISP : reload config
MUXHTTP --> HOOK : /hooks/* routes
@enduml
```

---

## Level 3: Component Diagram — Agent Layer

> Agent 如何处理 AI 调用？

```plantuml
@startuml c4-agent
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Agent (internal/agent)

interface "Agent\ninterface" as IFACE {
  +Chat([]Message) AgentResponse
  +Name() string
}

component "Router\nagent.go\n\nWraps the active agent.\nHolds per-session\nmodel overrides." as ROUTER

component "AnthropicClient\nanthropic.go\n\nBuilds HTTP requests.\nHandles API key vs\nOAuth auth modes.\nPosts to Claude API.\nParses SSE stream." as ACLIENT

component "FailoverAgent\nfaiover.go\n\nTries primary agent.\nOn error → tries\nfallback agent.\nLogs degradation." as FAILOVER

component "Summarizer\nsummarizer.go\n\nCalled by Compactor.\nSends old messages\nto Claude with\nsummarize instruction." as SUMM

component "Tool Registry\ntools/tools.go\n\nRegisters built-in tools.\nExec + web_search.\nCalled by agent on\nfunction_call blocks." as TOOLS

cloud "api.anthropic.com\nPOST /v1/messages\n(streaming SSE)" as ANT

ROUTER ..|> IFACE : implements
ACLIENT ..|> IFACE : implements
FAILOVER ..|> IFACE : implements

ROUTER --> FAILOVER : delegates
FAILOVER --> ACLIENT : primary
ACLIENT --> ANT : HTTPS request
ACLIENT --> TOOLS : execute tool calls
SUMM --> ACLIENT : uses for summarization

note right of ACLIENT
  Auth mode detection:
  sk-ant-oat → OAuth headers
  sk-ant-api → x-api-key
end note
@enduml
```

---

## Level 3: Component Diagram — Session & Memory

> 对话历史和记忆系统如何工作？

```plantuml
@startuml c4-session-memory
!theme plain
skinparam backgroundColor #FFFFFF
skinparam defaultFontName monospace

title Component Diagram — Session & Memory

component "Session Store\nsession/session.go" as STORE {
  component "Store\nmap[key]*Session" as MAP
  component "Session\nMessages []Message\nModel string" as SESS_OBJ
}

component "Compactor\nsession/compact.go\n\nEstimate tokens: len/4\nIf > threshold:\n  summarize old msgs\n  replace with Summary msg\n  keep last 10 intact" as COMP

component "Memory Manager\nmemory/memory.go\n\nLoads on startup.\nReloads on config change.\nBuilds layered prompt." as MEM

rectangle "Workspace Files\n(~/.feelpulse/workspace/)" as WORKSPACE #lightyellow {
  rectangle "SOUL.md\nPersona & identity\nReplaces base system\nprompt if present" as SOUL #fff2cc
  rectangle "USER.md\nUser context:\nname, timezone, prefs\nAppended to prompt" as USER_F #fff2cc
  rectangle "MEMORY.md\nLong-term memories:\ndecisions, events\nAppended to prompt" as MEM_F #fff2cc
}

MAP "1" --> "*" SESS_OBJ
SESS_OBJ --> COMP : when len(messages)\nexceeds 80k tokens

MEM --> SOUL : os.ReadFile()
MEM --> USER_F : os.ReadFile()
MEM --> MEM_F : os.ReadFile()

note right of MEM
  System prompt assembly:
  1. SOUL.md (if exists)
  2. config.agent.system
  3. "## User Context\n" + USER.md
  4. "## Memory\n" + MEMORY.md
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

## Level 4: Code — Message Processing Sequence

> 一条消息从 Telegram 到 Claude 再回到用户的完整代码路径

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
participant "command.go\nHandler" as CMD
participant "session.go\nStore" as SESS
participant "compact.go" as COMP
participant "memory.go\nManager" as MEM
participant "anthropic.go\nAnthropicClient" as ANT_CLIENT
participant "Anthropic API\napi.anthropic.com" as ANT
participant "usage.go\nTracker" as USAGE

TG -> BOT : {update_id, message: "hi"}
BOT -> GW : handleMessage(\n  channel="telegram"\n  userID="leagmain"\n  text="hi"\n)
activate GW

GW -> CMD : IsCommand("hi")
CMD -> GW : false

GW -> SESS : GetOrCreate("telegram:leagmain")
SESS -> GW : session{messages:[...history...]}

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
GW -> USAGE : Track("telegram:leagmain", 150, 12)

GW -> BOT : return reply message
deactivate GW

BOT -> TG : POST /sendMessage\n{chat_id, text:"Hi there!..."}
TG -> BOT : {ok:true, message_id:123}
@enduml
```

---

## Level 4: Code — Auth Flow

> 两种认证方式的代码差异

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

## 系统设计原则

| 原则 | 实现 |
|------|------|
| **单进程** | 所有组件在同一 Go 进程，零 IPC 开销 |
| **无持久化** | Session 在内存，重启清空（设计选择：简单 > 复杂） |
| **流式响应** | Claude SSE stream → 边生成边处理 |
| **可热重载** | config.yaml 改变 → watcher 检测 → 自动重载 |
| **分层 Prompt** | SOUL → base system → USER → MEMORY 依次叠加 |
| **自动压缩** | 超过 80k token → 旧消息摘要化，保持上下文可控 |
| **订阅认证** | 伪装 Claude Code headers → 用 Claude 订阅额度，不额外付费 |
| **3ms 启动** | Go 原生二进制，无 JVM/V8 启动开销 |
