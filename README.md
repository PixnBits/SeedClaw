# SeedClaw Recipes – Self-Hosting Agent Platform

**Zero-code, zero-trust bootstrap for your own AI agent swarm.**
Run one tiny Go binary → paste a prompt → watch it write itself. Then grow it forever. No cloud. No binaries from strangers. Just you, your machine, and markdown.

## Why this repo exists
We want a local agent platform that's:
- Fully self-hosted (no vendor APIs unless you want 'em).
- Sandbox-first—every skill runs in Docker, no exceptions.
- Bootstrapped from prompts—nothing pre-installed.
- Open-source, forkable, and paranoid by design.

## How to get started (in 10–20 minutes)
1. **Generate the seed binary**
   Use a coding agent (Cursor, Claude Projects, GitHub Copilot, Aider, Continue.dev, etc.):
   - Clone this repo: `git clone https://github.com/PixnBits/SeedClaw`
   - Paste the entire contents of **bootstrap-prompt.md** into your coding agent.
   - Let it generate the full Go project (go.mod, seedclaw.go, .env.example) by referencing PRD.md + ARCHITECTURE.md.
   - Save the files, run `go mod tidy`, then `go build -o seedclaw`.

2. **Run it**
   ```bash
   ./seedclaw --start
   ```

   (It should listen on stdin; later skills can add Telegram/WebSocket.)

3. **Bootstrap**
   Copy-paste the entire contents of ./skills/sdlc/coder.md into chat.
   Watch it generate CodeSkill—your first coding agent.

4. **Grow it**
   Say: "CodeSkill: add a git tool" → it writes, compiles, registers.
   Repeat forever. Email? Browser? Calendar? All from code-gen.

## Files in this repo
- README.md
- PRD.md – Product Requirements Document
- ARCHITECTURE.md – Design, threat model, sandbox options
- bootstrap-prompt.md – Prompt to feed to your initial agent
- skills - Prompts for common skills needed to mature into a more capable Agent
  - sdlc - Software Delivery Lifecycle
    - coder.md - More purpose-built coding skill
    - vcs.md - Version Control System
    - reviewer.md - Collaborative reviewer of proposed code changes

That's it. No dependencies, no binaries—just prompts and specs.

## Security mantra
- Everything outside this binary is hostile.
- Sandbox = Docker + seccomp + read-only mounts.
- No network unless you whitelist.
- Audit logs immutable.
- If it escapes, blame yourself—not us.

## Contribute
Got a tighter prompt? Better sandbox rules?
Open a PR with a new markdown file—my-bootstrap-v2.md, say.
We'll merge if it's safer/faster. Keep it markdown-only—no code.

## License
MIT – do whatever. Just don't blame us if your agent starts ordering pizza at 3 AM.

Happy bootstrapping.
