You are helping bootstrap SeedClaw – a paranoid, self-hosting local AI agent platform.

Goal: For each core skill (`messagehub`, `llmcaller`, `coder`, `skill-builder`), generate **in one shot**:
- A minimal Go `main` program implementing the skill.
- A `Dockerfile` that builds a static-ish binary using `golang:alpine` and runs it from `alpine:latest`.
- An updated markdown spec in `skills/*/[skill_name].md` that matches the orchestrator’s expectations.

Read these files for context (from this repo):
- ARCHITECTURE.md (v1.2) → thin host orchestrator + containerized skills + narrow IPC
- bootstrap-prompt.md → overall bootstrap goal and orchestrator behavior

Host orchestrator facts (seedclaw.go):
- Listens on Unix socket at `~/.seedclaw/ipc.sock` (0600) for IPC requests.
- Allowed IPC actions: `start_skill`, `stop_skill`, `restart_skill`, `get_logs`, `get_status`.
- Starts skills via Docker using config parsed from `skills/*/[skill].md` (`Image`, `Mount`, `Network`, `Command`).
- Communicates with skills by:
  - Writing JSON lines to container stdin via `docker exec -i <name> sh -c "cat"`.
  - Reading skill container logs (`stdout`/`stderr`) and interpreting **each line as one JSON object**.
- For user-visible messages, it expects lines like:
  `{ "to": "user", "payload": { "text": "..." } }`.

## Task

Generate a complete, minimal Go skill binary for **[SKILL_NAME]** that is compatible with this orchestrator.

### Common requirements for all skills

1. Package main with a simple long-running loop:
   - Use `bufio.NewScanner(os.Stdin)` to read line-delimited JSON messages from stdin.
   - For each line, unmarshal into a `map[string]interface{}` (or small struct) and handle according to the skill’s role.
   - Never exit on a single bad message – log an error as JSON and continue.

2. Logging / output:
   - All stdout lines must be **JSON objects**.
   - For messages intended for the user, emit:
     `{ "to": "user", "payload": { "text": "..." } }`.
   - For routing to other skills, emit JSON that includes at least `from`, `to`, `type`, and `payload` fields.

3. IPC client (optional but allowed where needed):
   - To talk back to the host IPC server, connect with `net.Dial("unix", "/ipc.sock")` (the host socket will be mounted there).
   - Send line-delimited JSON objects of the form `IPCRequest`:
     `{ "action": "start_skill" | "stop_skill" | "restart_skill" | "get_logs" | "get_status", ... }`.

4. Security:
   - No use of `syscall`, `unsafe`, `plugin`.
   - No `os/exec` inside skills.
   - No file writes outside `/tmp`.
   - Validate JSON before acting on it.

5. Dependencies:
   - Prefer stdlib (`bufio`, `encoding/json`, `net`, `os`, `time`, `log`, `net/http` when needed for LLMs).

### Skill-specific requirements

For `[SKILL_NAME]`, follow the semantics and constraints described in the existing markdown file:
- For `messagehub`: central message router, routes JSON messages based on `to` field, logs a startup message like:
  `{"to":"user","payload":{"text":"Messagehub started"}}`.
- For `llmcaller`: calls local Ollama on host network (e.g. `http://127.0.0.1:11434`) and optionally OpenAI-compatible API using env vars.
- For `coder`: generates safe Go code according to security rules, outputs JSON with `code`, `filename`, `explanation`.
- For `skill-builder`: compiles code in `/tmp`, runs basic tests, and optionally sends IPC `start_skill` requests for new skills.

### Dockerfile requirements

For each `[SKILL_NAME]`, generate a `Dockerfile` that:
- Uses a multi-stage build:
  - `FROM golang:1.23-alpine AS builder`
  - `WORKDIR /app`
  - `COPY [skill_name].go .`
  - `RUN go build -o [skill_name] [skill_name].go`
  - `FROM alpine:latest`
  - `WORKDIR /app`
  - `COPY --from=builder /app/[skill_name] /app/[skill_name]`
  - `ENTRYPOINT ["/app/[skill_name]"]`.
- Produces images with **exact** tags:
  - `seedclaw-messagehub:latest`
  - `seedclaw-llmcaller:latest`
  - `seedclaw-coder:latest`
  - `seedclaw-skill-builder:latest`.

### Markdown spec requirements

Also output an updated markdown spec for the skill at:
- `skills/core/[skill_name].md` for `messagehub`, `llmcaller`.
- `skills/sdlc/[skill_name].md` for `coder`, `skill-builder`.

Each spec must contain:
- A role section describing what the skill does.
- Input and output JSON formats.
- Security rules.
- A `**Docker run spec**` section with bullets **exactly** like:
  - `- Image: seedclaw-[skill_name]:latest`
  - `- Mount: $HOME/.seedclaw/ipc.sock:/ipc.sock:ro[, /tmp:/tmp:rw]` (coder and skill-builder also mount `/tmp`).
  - `- Network: none` for messagehub and skill-builder, `host` for llmcaller and coder.
  - `- Command: /app/[skill_name]`.

### Output format

Output **only** the following as fenced code blocks, in this order:
1. `[skill_name].go`          – the full main Go file.
2. `Dockerfile`               – multi-stage build as specified above.
3. `skills/.../[skill_name].md` – the updated markdown spec matching the orchestrator.

Generate for **[SKILL_NAME]** now.
