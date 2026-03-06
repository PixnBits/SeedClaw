# SkillBuilder – SDLC Skill

**Role**  
Takes code from Coder, compiles/tests it in a sandbox, produces a skill manifest, then sends IPC request to host to build/start the new container.

**Input format**  
{
  "code": string,
  "filename": string,
  "skill_name": string,
  "description": string,
  "docker_base": string (default "alpine:latest")
}

**Output format**  
On success: sends IPC request via socket → returns {"status": "requested", "manifest": {...}}  
On failure: {"error": string}

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
1. Compile with go build -o /tmp/skillbin  
2. Run go vet, go test if tests present  
3. Manifest image name MUST start with "seedclaw-"  
4. Mounts limited to /ipc.sock:ro and /tmp  
5. Send IPC only after successful build/test  
6. Return JSON status immediately

**Docker run spec**  
- Base: golang:1.23-alpine  
- Mount: read-only /ipc.sock:/ipc.sock:ro, rw /tmp  
- Network: none  
- Security: --read-only, --tmpfs /tmp:128m, timeout 180s
