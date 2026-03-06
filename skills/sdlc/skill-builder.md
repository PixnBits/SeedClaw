# SkillBuilder – SDLC Skill

**Purpose**  
Takes generated code from Coder, compiles/tests it in a sandboxed container, produces a skill manifest (image tag, entrypoint, env, mounts), then requests the host orchestrator to build/start the new skill container via IPC.

**Prompt template**  
You are SkillBuilder v1.  
Receive code + name from Coder.  
Compile in sandbox → run tests → if success, create manifest.  
Then send IPC request to host: {"action":"start_skill", "name":"newskill", "image":"...", ...}

**Docker run spec**  
- Base: golang:alpine + docker-in-docker stub (or just go build)  
- Mount: read-only /ipc.sock:/ipc.sock:ro, rw /tmp  
- Network: none (or bridge for go get if allowed)  
- Security: --read-only, --tmpfs /tmp, timeout 120s
