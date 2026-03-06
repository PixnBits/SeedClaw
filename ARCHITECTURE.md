# SeedClaw Architecture

**Version:** 1.2 – Thin Host + Controlled IPC (2026-03)
**Status:** Refined bootstrap phase – working toward MVP implementation

## Core Principles
- The **host binary** (`seedclaw`) is the only trusted code running on bare metal.  
  It is deliberately dumb: chat listener + message router + container lifecycle manager + narrow IPC server.
- All intelligence, LLM calls, code generation, compilation, and skill registration live **inside containerized skills**.
- No LLM interaction, no code compilation, no arbitrary exec on the host.
- Self-modification (adding/changing skills) is confined to containers.
- Repo contains **only markdown documentation** — users generate the orchestrator + core skills using a coding agent.

## Components

### Host Orchestrator (seedclaw binary)
Responsibilities (only):
- Accept user input (stdin loop minimum; Telegram bot or WebSocket preferred)
- Route messages between user and skills via pub/sub (in-memory or via MessageHub)
- Manage container lifecycle (start, stop, restart, logs, status, remove)
- Expose a **narrow, validated IPC API** via Unix domain socket (filesystem or abstract)
- Maintain minimal persistent state: ~/.seedclaw/registry.json (skill name → container config)

**IPC API (critical security surface)**
- Unix socket at `~/.seedclaw/ipc.sock` (0600 permissions) or abstract `@seedclaw-ipc`
- Protocol: line-delimited JSON requests/responses
- Allowed actions (strictly validated):
  - start_skill {name, image, env, mounts, cmd}
  - stop_skill {name}
  - restart_skill {name}
  - get_logs {name, lines}
  - get_status {name}
- All parameters sanitized/whitelisted (e.g. image must match known prefix or hash)
- Peer credential check (SO_PEERCRED) to confirm connector UID matches host user
- Rate limiting + timeout on requests

### Core Skills (all containerized from day 1)
1. `core/messagehub`     — central pub/sub router, forwards messages between skills & user
2. `core/llmcaller`      — safe interface to local Ollama / API LLMs, sanitizes inputs/outputs
3. `sdlc/coder`          — generates and edits safe Go code for new skills
4. `sdlc/skill-builder`  — compiles/tests code in its own container, produces manifest, requests host to start new skill via IPC

Additional skills (git, email, browser, etc.) are generated later via skill-builder.

## Sandbox & Isolation
- Every skill runs in its own Docker container (alpine base recommended)
- Default flags: --read-only, --network=none (except llmcaller may need host network for Ollama), --cap-drop=ALL, cgroup limits, timeout
- Future: gVisor (runsc runtime), Firecracker microVMs
- **No skill container ever receives the Docker socket** — lifecycle control stays in host

## Bootstrap Flow
1. Feed bootstrap-prompt.md into Copilot / Claude / Cursor / Grok / Aider
2. Agent generates:
   - go.mod + seedclaw.go (thin orchestrator with IPC listener)
   - Four core skill markdown files (prompt + Docker spec)
3. User builds & runs orchestrator
4. Orchestrator launches the four core skills
5. Platform is live → user can now ask skill-builder to create new skills

## Threat Model Highlights
- Compromised skill → cannot escape container or control Docker daemon
- Malicious IPC request → rejected by strict validation in host
- Same-user attacker → can connect to IPC but only use whitelisted actions
- Root attacker → game over anyway (assume trusted machine)

See PRD.md for full scope and non-goals.