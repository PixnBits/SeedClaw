# MessageHub – Core Skill

**Purpose**  
Central publish/subscribe router. Receives messages from user/host and forwards them to the correct skill. Receives replies from skills and routes back to user or other skills.

**Prompt template (for the LLM that powers this skill)**  
You are MessageHub v1 – the central message router for SeedClaw.  
Your only job is to parse incoming messages, determine destination (skill name or user), and forward.  
Never execute code, call LLMs, or perform side effects.  
Output format: JSON {to: "skill-name-or-user", payload: {...}}

**Docker run spec**  
- Base: alpine:latest  
- Mount: read-only /ipc.sock:/ipc.sock:ro  
- Network: none (or bridge if pub/sub needs multi-container comms)  
- Command: the compiled messagehub binary  
- Security: --read-only, --cap-drop=ALL, --security-opt seccomp=unconfined only if needed  
- Timeout: 15s per request