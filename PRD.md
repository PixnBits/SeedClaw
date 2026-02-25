# SeedClaw Product Requirements Document (PRD)

**Version:** 1.0-draft (2026-02-25)  
**Status:** Bootstrap phase – focus on self-initialization

## 1. Overview & Mission
SeedClaw is a **minimal, local-first, self-bootstrapping AI agent platform**.  
Users run one small Go binary they compile themselves → feed it a prompt → the system uses an LLM to generate, compile (in sandbox), test, and register its own extensions ("skills").  
No cloud dependency, no pre-built binaries in the repo, no vendor lock-in. Everything after the seed is emergent and AI-generated.

Core tagline: "Bootstrap your own paranoid agent swarm from markdown prompts only."

## 2. Key Requirements & Constraints
- **Local-only execution** — runs on user machine (Mac/Windows/Linux, x86/arm).
- **LLM integration** — prefer local (Ollama, LM Studio, llama.cpp); fallback to API (Claude, Grok, OpenAI, etc.) via env var config.
- **Chat input** — at minimum stdin/stdout loop; nice-to-have: Telegram bot (via BOT_TOKEN env), WebSocket server.
- **Sandbox mandatory** — every code gen, compile, test, and skill execution in isolated environment (Docker default; gVisor/Firecracker future).
- **No persistent external state** (except optional ~/.seedclaw/ for skill registry and audit logs).
- **Security paranoia** — treat all LLM output as hostile; static analysis + strict sandboxing.
- **Repo purity** — this repo contains **only markdown** (prompts, docs). Users generate seedclaw.go themselves.

## 3. MVP Scope (what the initial seed binary must do)
The seed binary (seedclaw) is the **only trusted component**. It must:
- Accept user prompts (stdin at minimum; Telegram/WebSocket preferred).
- Call an LLM with a structured prompt + context.
- Parse structured output from LLM (e.g. JSON: {code: string, binary_name: string, hash: string}).
- Spawn Docker container to:
  - Compile generated Go code (using alpine + golang image or local toolchain).
  - Run go vet / basic lint.
  - Test with simple "hello world" invocation.
- Register successful skills in-memory (or to file): map of name → {prompt_template, binary_path}.
- Execute registered skills on future user requests, always in fresh sandbox.
- Log all actions immutably (stdout + optional audit file).

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
1. Generate/compile seedclaw.go (using this PRD + ARCHITECTURE.md + bootstrap-prompt.md in a coding agent).
2. Run `./seedclaw --start`.
3. Paste bootstrap-prompt.md content into the interface.
4. Receive a working "CodeSkill" that can then generate further skills.

See ARCHITECTURE.md for detailed design, threat model, and sandbox evolution path.

Contributions: Improve this PRD, ARCHITECTURE.md, or the prompts via PR.
