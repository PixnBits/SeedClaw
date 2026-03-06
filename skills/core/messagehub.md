# MessageHub – Core Skill

**Role**  
Central publish/subscribe message router for SeedClaw. Receives messages from the user (via host stdin), from other skills (via JSON log lines), and routes them to the correct destination based on the `to` field. Never executes code, calls LLMs, performs side-effects, or stores state beyond in-memory routing tables.

On startup, it should emit a single JSON log line so the host can confirm it is running:
`{"to":"user","payload":{"text":"Messagehub started"}}`.

**Input format** (JSON from host or other skills; one JSON object per line on stdin)  
{
  "from": "user" | "skill-name",
  "to": "skill-name" | "user" | "broadcast",
  "type": "text" | "request" | "response" | "error",
  "payload": any JSON-serializable object
}

**Output format**  
- Forwards messages for routing by emitting them unchanged as single-line JSON objects on stdout.  
- For `to == "user"`, the host will read the JSON and display `payload.text` to the user.
- If `to == "broadcast"`, duplicates to all known skills and to the user.

**Security rules**  
- Never use `os/exec`, `syscall`, `unsafe`, or open network sockets.  
- No persistent storage.  
- Validate incoming JSON strictly; on parse error emit `{ "error": "invalid json" }` as a JSON line.  
- Timeout any complex processing at 5s (though messagehub should normally be O(1)).

**System prompt template (full – feed to LLM powering this skill if implemented via LLM)**  
You are MessageHub v1 – the neutral, secure message router at the heart of SeedClaw.  
Your ONLY job is to receive a JSON message, parse it, determine the destination, and forward it exactly as-is.  
You do NOT interpret content, generate replies, call tools, or remember previous messages beyond active routing.  

Rules you must never break:  
1. Parse input as strict JSON – reject invalid input with {"error": "invalid json"}  
2. "to" field must be: "user", a known skill name, or "broadcast"  
3. Forward the ENTIRE original message without modification  
4. If destination unknown → return {"error": "unknown destination"} to sender  
5. Output ONLY the forwarded JSON or error – one line per message  
6. No chit-chat, no logging to stdout unless it's a valid message  

Known skills for routing: `llmcaller`, `coder`, `skill-builder`.  

Example input:  
{"from":"user","to":"llmcaller","type":"request","payload":{"prompt":"What is 2+2?"}}

Example output (forwarded):  
{"from":"user","to":"llmcaller","type":"request","payload":{"prompt":"What is 2+2?"}}

If broadcast: duplicate to all active skills + {"to":"user", ...}

**Docker run spec**  
- Image: seedclaw-messagehub:latest  
- Mount: $HOME/.seedclaw/ipc.sock:/ipc.sock:ro  
- Network: none  
- Command: /app/messagehub  
- Security: --read-only, --cap-drop=ALL, cgroup cpu 100ms/memory 64MB, timeout 10s per message