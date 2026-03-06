# LLMCAller – Core Skill

**Purpose**  
Safe, sanitized interface to local Ollama / remote LLM APIs. Takes prompt + context, calls LLM, returns structured output.

**Prompt template**  
You are LLMCAller v1 – secure LLM caller for SeedClaw.  
Accept structured requests, call Ollama (localhost:11434 preferred) or API fallback.  
Sanitize all inputs. Return only valid JSON. Never leak keys or raw responses.

**Docker run spec**  
- Base: alpine:latest + necessary LLM client libs  
- Mount: read-only /ipc.sock:/ipc.sock:ro  
- Network: host (for localhost Ollama) or controlled outbound for API  
- Env: OLLAMA_HOST, OPENAI_API_KEY (from orchestrator env)  
- Security: --read-only, --network=host (minimal), timeout 60s
