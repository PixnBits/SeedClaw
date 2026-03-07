# SeedClaw Product Requirements Document (PRD)

**Version:** 1.1 (2026-03-07)  
**Status:** Bootstrap phase – minimal committed core for reliable first run

## 1. Overview & Mission

SeedClaw is a minimal, local-first, self-bootstrapping AI agent platform.  
After early experiments showed pure zero-code bootstrap to be too fragile for most users, the system now ships a small, auditable core that gets the user to a working state in one step.  
Everything beyond that core (new tools, agents, capabilities) is generated, compiled in sandbox, and dynamically added by the system itself.

**Tagline:** "Bootstrap your own paranoid agent swarm from markdown prompts only."

## 2. Key Requirements & Constraints

- Completely local execution (Mac / Windows / Linux, x86 / arm)  
- LLM integration: prefer local Ollama / llama.cpp; fallback to API via config  
- Chat input: stdin/stdout loop minimum; WebSocket server or Telegram bot optional  
- Every skill executes inside its own Docker container  
- No persistent external state except `~/.seedclaw/` (skill registry, audit logs, compose.yaml backups)  
- Security paranoia: treat all LLM output and generated code as hostile  
- Repository contains **only** the minimal trusted starter pieces:  
  ```
  /README.md
  /ARCHITECTURE.md
  /PRD.md
  /src/seedclaw.go
  /src/skills/core/message-hub/{SKILL.md, messagehub.go, Dockerfile}
  /src/skills/core/llm-caller/{SKILL.md, *.go, Dockerfile}
  /src/skills/core/ollama/{SKILL.md, *.go, Dockerfile}
  /src/skills/sdlc/coder/{SKILL.md, *.go, Dockerfile}
  /compose.yaml
  ```

## 3. MVP Scope – What the initial seed binary must do

The seed binary (`seedclaw`) is the only component that runs directly on the host. It must:

- Be compilable from `./src/seedclaw.go` with `go build`  
- On first run (`./seedclaw --start`):  
  - Verify Docker is available  
  - Start the four core services defined in `compose.yaml`  
  - Create a Unix socket (e.g. `/run/seedclaw.sock`) and mount it read-write into the `message-hub` container  
- Accept user prompts (stdin minimum)  
- Route **all** LLM calls and skill interactions through `message-hub`  
- When a new skill should be created:  
  1. Send generation request via message-hub to `coder` skill  
  2. Receive proposed `SKILL.md` + Go code + `Dockerfile`  
  3. Compile & basic-test in a temporary isolated container  
  4. If successful: append new service definition to `compose.yaml`  
  5. Run `docker compose up -d` to start the new skill  
  6. Register skill in persistent registry  
- Maintain immutable logging of all actions (stdout + optional file)

## 4. Non-Goals (MVP)

- Multi-user support / authentication  
- GUI or web dashboard  
- Built-in multi-agent orchestration primitives (leave to generated skills)  
- Cloud deployment / hosted service  
- Automatic skill versioning / revocation (defer to later generated capabilities)

## 5. Minimal Dependencies

- Go standard library  
- Docker client library (`github.com/docker/docker/client`)  
- Optional: websocket or telegram-bot-api library (only if chat interface is extended)  
- LLM client logic lives **inside** the `llm-caller` skill container

## 6. Success Criteria for First Run

User can:

1. Clone the repo  
2. `go build -o seedclaw ./src/seedclaw.go`  
3. `./seedclaw --start`  
4. Observe the four core containers start via Docker Compose  
5. Send a prompt (e.g. paste content of a bootstrap prompt file)  
6. Immediately have access to the `coder` skill to generate new capabilities

See ARCHITECTURE.md for detailed threat model, communication flow, sandbox rules, and evolution path.

Contributions welcome: improve docs, core skill implementations, or bootstrap prompts.
