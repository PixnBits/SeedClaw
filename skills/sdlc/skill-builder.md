# SkillBuilder – SDLC Skill

**Role**  
Takes code from `coder`, compiles/tests it in a sandbox, produces a skill manifest, then (optionally) sends IPC requests to the host to build/start the new container or register it.

**Input format** (payload carried inside routed messages)  
Inbound messages are JSON lines of the form:
{
  "from": "coder" | "user",
  "to": "skill-builder",
  "type": "response" | "request",
  "payload": {
    "code": string,
    "filename": string,
    "skill_name": string,
    "description": string,
    "docker_base": string (default "alpine:latest")
  }
}

**Output format**  
Single-line JSON objects on stdout, typically routed back to the user or orchestrator via `messagehub`, e.g.:
- On success: `{ "from": "skill-builder", "to": "user", "type": "response", "payload": { "status": "requested", "manifest": { ... } } }`  
- On failure: `{ "from": "skill-builder", "to": "user", "type": "error", "payload": { "error": "..." } }`

**Manifest example**  
{
  "name": "echo-skill",
  "image": "seedclaw-echo:v1",
  "entrypoint": ["/app/echo"],
  "env": {"KEY": "val"},
  "mounts": ["/ipc.sock:/ipc.sock:ro"],
  "network": "none"
}

**System prompt template**  
You are SkillBuilder v1 – the secure compiler and launcher assistant for SeedClaw.  
Receive code → compile in sandbox → run basic test → if OK, construct manifest → send IPC {"action":"start_skill", ...} to host.

Rules:  
1. Compile with `go build -o /tmp/skillbin` inside the container.  
2. Optionally run `go vet` and `go test` if tests are present.  
3. Manifest image name MUST start with `seedclaw-`.  
4. Mounts limited to `/ipc.sock:ro` and `/tmp` in manifests.  
5. Send IPC only after successful build/test by dialing `/ipc.sock` and sending an `IPCRequest` JSON line.  
6. Always emit a JSON status line on stdout immediately so the user/orchestrator can see progress.

**Docker run spec**  
- Image: seedclaw-skill-builder:latest  
- Mount: $HOME/.seedclaw/ipc.sock:/ipc.sock:ro, /tmp:/tmp:rw  
- Network: none  
- Command: /app/skill-builder  
- Security: --read-only, --tmpfs /tmp:128m, timeout 180s
