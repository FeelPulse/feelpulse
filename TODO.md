# FeelPulse TODO

## ✅ Completed (2025-02-17)

### Core Features
- ✅ Telegram Bot — Long polling with Markdown support
- ✅ Anthropic Claude — Native Messages API client
- ✅ HTTP Gateway — Health checks and webhooks
- ✅ YAML Config — Simple configuration
- ✅ Session Persistence — SQLite-backed conversation history
- ✅ Memory/Workspace — SOUL.md, USER.md, MEMORY.md support
- ✅ Context Compaction — Auto-summarize long conversations
- ✅ Config Hot Reload — Apply changes without restart
- ✅ TUI — Terminal chat interface (bubbletea)
- ✅ Telegram Bot Menu — Inline keyboards and command menu

### New Features (Wave 4)
- ✅ TTS Voice Support — Auto-detects espeak/say/festival
  - `/tts on/off` commands
  - Text sanitization (removes markdown/emoji)
  - Per-session toggle
- ✅ Personality Profiles — Multiple SOUL.md variants
  - `/profile list`, `/profile use <name>`
  - Config: workspace.profiles map
- ✅ Improved Reminders
  - `/remind at HH:MM` absolute time support
  - `/cancel <id>` to cancel reminders
  - SQLite persistence (survives restarts)
  - Better time display ("in 23 min")

### Infrastructure
- ✅ systemd Service — install-service, enable-service make targets
- ✅ Skills System — SKILL.md extensible tools
- ✅ Heartbeat — Periodic proactive checks
- ✅ Multi-user Allowlist — Telegram security
- ✅ OpenAI-compatible API — /v1/chat/completions endpoint
- ✅ Web Dashboard — Simple status page
- ✅ Rate Limiting — Per-user message throttling
- ✅ SQLite Store — Session and reminder persistence
- ✅ Usage Tracking — Token usage per session

## 📋 Backlog

### Stretch Goals
- [ ] Discord channel support (basic implementation exists)
- [ ] Plugin system (dynamic loading)
- [ ] Browser control
- [ ] Sub-agent / isolated sessions
- [ ] Voice input (speech-to-text)
- [ ] MCP (Model Context Protocol) support
- [ ] Multi-model routing (use different models for different tasks)
- [ ] Conversation export to multiple formats (JSON, PDF)
- [ ] Web UI dashboard improvements

### Nice to Have
- [ ] Prometheus metrics endpoint
- [ ] Docker image
- [ ] ARM64 builds
- [ ] Config encryption for secrets
- [ ] Backup/restore commands
- [ ] Message scheduling (send messages at specific times)
