You are helping bootstrap SeedClaw – a paranoid, self-hosting local AI agent platform.
Repo (thin-orchestrator-ipc branch): https://github.com/PixnBits/SeedClaw
Generate each skill in this order: `messagehub`, `llmcaller`, `coder`, and `skill-builder`

Read these files for context:
- ARCHITECTURE.md (v1.2) → thin host orchestrator + containerized skills + narrow IPC
- bootstrap-prompt.md → overall bootstrap goal
- The running seedclaw.go binary uses:
  - Unix socket at ~/.seedclaw/ipc.sock (0600)
  - Allowed IPC actions: start_skill, stop_skill, restart_skill, get_logs, get_status
  - Skills are started via Docker with config parsed from skills/*/[skill].md
  - Communication to skills uses docker exec -i to push JSON to stdin (fragile – we'll improve later)

Task: Generate a complete, minimal Go skill binary for [SKILL_NAME]

Requirements:
1. Package main – simple stdin/stdout loop or long-running service
2. Connect to the host IPC socket (/ipc.sock) as client (net.Dial("unix", "/ipc.sock"))
3. Use the narrow IPC protocol: send line-delimited JSON requests (only allowed actions)
4. Listen on stdin for incoming messages (JSON pushed via docker exec)
5. Route / process messages according to the role below
6. Send replies / requests back via stdout (host reads logs) OR via IPC socket when appropriate
7. Strict security: no os/exec (except if whitelisted), no syscall, no unsafe, no file writes outside /tmp, validate all input
8. Output format: always structured JSON on stdout when replying to user or other skills
9. Dependencies: minimal (net, encoding/json, bufio, etc.)

Specific role for [SKILL_NAME]:
[PASTE FULL improved markdown from earlier conversation here – e.g. the expanded messagehub.md content]

Output ONLY the following as fenced code blocks:
- skill_name.go          (the full main Go file)
- Dockerfile             (minimal, based on alpine + golang if needed)
- updated skills/*/[skill_name].md   (enhanced version with better prompt template, input/output schemas, Docker spec)

After this skill is generated, I will:
- Place [skill_name].go in a `src/[skill_name]` dir
- Build it (`$ cd src/[skill_name] && docker build -t seedclaw/[skill_name] .`)
- Create a Docker image or run directly with config from .md
- Ask the orchestrator to start it via IPC or manual start

Generate for [SKILL_NAME] now.
