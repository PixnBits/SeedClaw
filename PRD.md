# SeedClaw Product Requirements Document (PRD)

**Version:** 2.0-draft (2026-03-06)  
**Status:** Thin-orchestrator + core skills bootstrap complete; next phase is self-hosted skill generation

## 1. Overview & Mission
SeedClaw is a **minimal, local-first, self-bootstrapping AI agent platform**.  
Users run one small Go orchestrator binary they compile themselves → that binary starts a small set of containerized "core skills" → those skills (powered by an LLM) generate, compile (in sandbox), test, and register further skills.  
No cloud dependency, no pre-built binaries in the repo, no vendor lock-in. Everything after the seed + four core skills is emergent and AI-generated.

Core tagline: "Bootstrap your own paranoid agent swarm from markdown prompts only."

## 2. Key Requirements & Constraints
- **Local-only execution** — runs on user machine (Mac/Windows/Linux, x86/arm).
- **LLM integration** — prefer local (Ollama, LM Studio, llama.cpp); fallback to API (Claude, Grok, OpenAI, etc.) via env var config.
- **Chat input** — at minimum stdin/stdout loop in the host orchestrator; nice-to-have: Telegram bot (via `TELEGRAM_BOT_TOKEN` env), WebSocket server.
- **Sandbox mandatory** — every code gen, compile, test, and skill execution in isolated environment (Docker default; gVisor/Firecracker future).
- **No persistent external state** (except optional ~/.seedclaw/ for skill registry and audit logs).
- **Security paranoia** — treat all LLM output as hostile; static analysis + strict sandboxing.
- **Repo purity** — this repo contains **only markdown** (prompts, docs). Users generate seedclaw.go themselves.

## 3. MVP Scope (what the initial seed binary must do)
The seed binary (`seedclaw`) is the **only trusted host component**. In the current thin-orchestrator architecture it must:
- Accept user prompts (stdin at minimum; Telegram/WebSocket optional).
- Manage **Docker lifecycle** for skills:
  - Parse markdown specs in `skills/core/*.md` and `skills/sdlc/*.md`.
  - Start one container per core skill using the image, mounts, command, and network defined in each markdown file.
  - Provide a narrow Unix socket IPC server on `~/.seedclaw/ipc.sock` with actions: `start_skill`, `stop_skill`, `restart_skill`, `get_logs`, `get_status`.
- Handle **message routing and chat loop**:
  - Read user text lines from stdin.
  - Wrap each into a JSON message and send to the `messagehub` skill via `docker exec -i <container> sh -c "cat"`.
  - Follow container logs for all skills; print `payload.text` for any JSON line where `to == "user"`.
- Enforce sandbox constraints for all skills via Docker:
  - Use read-only root filesystems where possible.
  - Mount only `~/.seedclaw/ipc.sock` and `/tmp` (for build/test skills) into containers.
  - Restrict networks (`none` for pure internal skills; `host` only where LLM access is required).

The **LLM integration, code generation, compilation, and dynamic skill registration** are implemented inside containerized skills (not in the host binary):
- `llmcaller` → calls local or remote LLMs safely.
- `coder` → generates Go code under strict security rules.
- `skill-builder` → compiles/tests code in `/tmp` and requests new skills to be started via IPC.

## 4. Non-Goals (MVP)
- Multi-user support.
- GUI / web UI.
- Built-in multi-agent orchestration.
- Cloud hosting / deployment.
- Skill revocation / versioning (add later via generated skills).

## 5. Dependencies (minimal)
- Go stdlib only where possible.
- External: docker client lib (github.com/docker/docker/client), websocket or telegram lib if chosen.
- LLM client: ollama-go or openai-compatible lib (configurable).

## 6. Success Criteria for Bootstrap
User can:
1. Generate/compile `seedclaw.go` (using this PRD + ARCHITECTURE.md + bootstrap-prompt.md in a coding agent).
2. Generate the four core skill markdown specs and skill code/Dockerfiles using `bootstrap-prompt.md` and `bootstrap-skills-prompt.md`.
3. Build core skill images:
  - `seedclaw-messagehub:latest`
  - `seedclaw-llmcaller:latest`
  - `seedclaw-coder:latest`
  - `seedclaw-skill-builder:latest`
4. Run `./seedclaw`.
5. Type into stdin and observe that:
  - The four core skill containers start and stay running.
  - `messagehub` logs a startup message and routes messages.
  - `llmcaller` can call a local LLM (e.g., Ollama) when configured.
  - `coder` and `skill-builder` can, together, generate, compile, and start a simple new skill through IPC.

See ARCHITECTURE.md for detailed design, threat model, and sandbox evolution path.

Contributions: Improve this PRD, ARCHITECTURE.md, or the prompts via PR.
