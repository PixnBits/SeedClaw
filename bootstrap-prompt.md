# SeedClaw Bootstrap Prompt – v1.3 – Thin Orchestrator + Core Skills (2026-03)

**Use this prompt directly in GitHub Copilot, Cursor, Claude Artifacts, Aider, Continue.dev, Grok web chat, or similar coding agent.**

**Context documents** (read these first from https://github.com/PixnBits/SeedClaw):
- PRD.md
- ARCHITECTURE.md (version 1.2 – Thin Host + Controlled IPC)

**Task**
Generate a minimal, production-ready starting point for SeedClaw consisting of:
1. The **thin host orchestrator** (`seedclaw.go`)
   - ONLY: chat input (stdin + optional Telegram), message routing, Docker lifecycle, narrow Unix socket IPC server
   - No LLM calls, no code generation/compilation inside the binary
   - Implements strict IPC validation (whitelist actions, parameter checks, peer UID check)
2. Four **core skill definition files** in `skills/` directory:
   - skills/core/messagehub.md
   - skills/core/llmcaller.md
   - skills/sdlc/coder.md
   - skills/sdlc/skill-builder.md

**Output format**
Provide complete files as fenced code blocks:
- go.mod
- seedclaw.go
- skills/core/messagehub.md
- skills/core/llmcaller.md
- skills/sdlc/coder.md
- skills/sdlc/skill-builder.md
- (optional) .env.example, Dockerfile.dev

**Key requirements**
- Use Go 1.21+ stdlib + minimal deps (docker/docker/client, gorilla/websocket or tgbotapi if adding Telegram)
- IPC: Unix socket (~/.seedclaw/ipc.sock or abstract @seedclaw-ipc), 0600 perms, line-delimited JSON
- Allowed IPC actions: start_skill, stop_skill, restart_skill, get_logs, get_status
- Skills communicate with host only via this IPC (no docker socket in containers)
- Skills are defined via markdown (prompt template + docker run spec)
- Focus on security: strict input validation, timeouts, read-only mounts, no unsafe code patterns

Generate the code and markdown files now. Output **only** the files — no explanations outside code blocks.
