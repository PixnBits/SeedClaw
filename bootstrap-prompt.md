# SeedClaw Bootstrap Prompt – v1.1 (2026-02-25)

Use this prompt **directly in GitHub Copilot, Cursor, Claude Artifacts, Aider, Continue.dev, or similar coding agent** to initialize the Go project and generate `seedclaw.go`.

**Instructions for the coding agent:**

You are generating the **initial seed binary** for SeedClaw – a self-hosting AI agent platform.  
Read and strictly follow these documents from the repo https://github.com/PixnBits/SeedClaw:

- PRD.md (Product Requirements Document) – defines mission, MVP scope, constraints.
- ARCHITECTURE.md – details principles, components, sandbox model, threat model.

**Task:**
Generate a complete, minimal, compilable Go project that implements the **seed binary** as described.

**Requirements for seedclaw.go:**
- Package main
- Use only necessary external modules (go mod init seedclaw; go get as needed)
- Minimal dependencies: 
  - LLM: prefer ollama-go (local) or openai-compatible client (API fallback via env OPENAI_API_KEY or similar)
  - Docker: github.com/docker/docker/client
  - Chat: start with bufio stdin/stdout loop; bonus: add telegram-bot-api if BOT_TOKEN env present
- Must implement the bootstrap loop:
  1. Accept user prompt (stdin or chat).
  2. Call LLM with the full user prompt + context (include safety rules from ARCHITECTURE.md).
  3. Expect structured JSON output from LLM: {code: string, binary_name: string, hash: string}
  4. Spawn Docker container (alpine + golang) to compile the code → run go vet → test execution.
  5. If successful, "register" the skill (in-memory map or simple file).
  6. Reply to user with status.
- Enforce security:
  - Docker run flags: --read-only, --network=none (default), --cap-drop=ALL, --security-opt seccomp=unconfined only if needed, timeout.
  - No os/exec outside sandbox.
  - Basic prompt guard: reject code with syscall, unsafe, os/exec unless whitelisted.
- Output: full project files as code blocks:
  - go.mod
  - go.sum (if any)
  - seedclaw.go (main logic)
  - Optional: .env.example, Dockerfile for dev

After generation, the user will build & run it, then paste this same prompt (or a variant) into the running binary to bootstrap CodeSkill.

Generate the code now. Output only the files—no explanations outside code blocks.
