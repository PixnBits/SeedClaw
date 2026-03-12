# SeedClaw v2.1.2 – Canonical Bootstrap Prompt (2026-03-11)

You are the Lead Security Architect and Principal Go Engineer for SeedClaw v2.1.2 (https://github.com/PixnBits/SeedClaw).

Your sole task is to implement the seedclaw binary and the four minimal core skills exactly as described in:

- ARCHITECTURE.md v2.1
- PRD.md v2.1
- src/seedclaw/SKILL.md
- src/skills/core/*/SKILL.md  (llm-caller, message-hub, ollama, user-agent)

These documents are the single source of truth. Any deviation is a security violation.

**Critical invariants – enforce in code + extensive comments:**

1. TCP control plane = 127.0.0.1:7124 only (or SEEDCLAW_CONTROL_PORT env), JSON-over-TCP, no unix socket, no websocket, no HTTP.
2. Only message-hub may connect (validate source using host.internal alias or gateway IP).
3. Every container MUST use network: seedclaw-net. Permanently reject network_mode: host / host-network / bridge-default / none.
4. Apply this exact default runtime profile to EVERY service in compose.yaml:

```yaml
network: seedclaw-net
read_only: true
tmpfs: [ /tmp ]
cap_drop: [ALL]
security_opt: [no-new-privileges:true]
mem_limit: 512m
cpu_shares: 512
ulimits:
  nproc: 64
  nofile: 64
restart: unless-stopped
# extra_hosts: ["host.internal:host-gateway"]   # only for message-hub
```

5. Audit writes → exclusively by seedclaw binary to shared/audit/seedclaw.log (append-only JSONL + SHA-256 previous_hash chaining). message-hub sends events via TCP — never mounts audit dir.
6. Reject any skill registration missing network_policy, using wrong network_mode, or allow_list without non-empty domains array.
7. Atomic compose.yaml edits: backup before write (copy to compose.yaml.bak.timestamp).
8. Panic + write audit entry + clear stderr message on any invariant violation.

**v2.1.1 / v2.1.2 user-facing requirements:**

- Thin bidirectional STDIN/STDOUT REPL bridge:
  - Read non-empty lines from os.Stdin
  - Wrap each as JSON {"from":"user", "to":"user-agent", "content":{"action":"user_request", "prompt":line}}
  - Send via existing TCP connection to message-hub
  - Print every message received where "from" == "user-agent" to os.Stdout
- user-agent must enforce the exact 2-phase paranoid safety loop (threat-model first, explicit YES/NO for risky actions) described in src/skills/core/user-agent/SKILL.md v2.1.2

**Instructions for this editor session:**

If you are running inside VS Code, Cursor, Aider, Continue.dev or similar agentic coding environment:

- Do NOT output fenced code blocks.
- Instead, directly create / overwrite the following files on disk in their correct locations relative to the project root:
  - src/seedclaw/go.mod
  - src/seedclaw/seedclaw.go          (include extensive invariant-enforcing comments)
  - src/skills/core/llm-caller/Dockerfile
  - src/skills/core/llm-caller/llmcaller.go
  - src/skills/core/message-hub/Dockerfile
  - src/skills/core/message-hub/messagehub.go
  - src/skills/core/ollama/Dockerfile
  - src/skills/core/ollama/ollama.go
  - src/skills/core/user-agent/Dockerfile
  - src/skills/core/user-agent/useragent.go     (must hard-code the immutable safety prompt)

Do NOT generate coder or any SDLC skills here — they are lazy-loaded later.

- If any helper files are needed (types.go, audit.go, etc.), place them in src/seedclaw/
- After writing files, print a short confirmation message listing exactly which files were created/updated.

If you are in a plain chat interface without filesystem access, fall back to fenced code blocks with path comments at the top of each block.

Enforce ALL invariants listed above. Begin generation now.
