# SeedClaw Recipes – Self-Hosting Agent Platform

**Zero-code, zero-trust bootstrap for your own AI agent swarm.**  
Run one tiny Go binary → paste a prompt → watch it write itself. Then grow it forever. No cloud. No binaries from strangers. Just you, your machine, and markdown.

## Why this repo exists
We want a local agent platform that's:  
- Fully self-hosted (no vendor APIs unless you want 'em).  
- Sandbox-first—every skill runs in Docker, no exceptions.  
- Bootstrapped from prompts—nothing pre-installed.  
- Open-source, forkable, and paranoid by design.

## How to get started (in 5 minutes)
1. **Build the seed binary**  

   ```bash
   # Clone this repo  
   git clone https://github.com/yourname/seedclaw-recipes  
   cd seedclaw-recipes  

   Write a minimal Go file (or copy from below)  
   cat <<EOF > seedclaw.go  
   package main  
   import "fmt"  
   func main() { fmt.Println("SeedClaw v1 – paste bootstrap prompt now") }  
   EOF  

   Build it (you'll replace this later with real code)  
   go build -o seedclaw  
   ```
   _(Pro tip: the real seed binary will be ~10KB—WebSocket + Docker client + LLM wrapper. But start dumb.)_

2. **Run it**
   ```bash
   ./seedclaw --start
   ```
   It'll spin up a WebSocket server + Telegram bot (configure your token in .env).

4. **Bootstrap**  
   Copy-paste the entire contents of ./bootstrap-prompt.md into chat.  
   Watch it generate CodeSkill—your first coding agent.

5. **Grow it**  
   Say: "CodeSkill: add a git tool" → it writes, compiles, registers.  
   Repeat forever. Email? Browser? Calendar? All from code-gen.

## Files in this repo
- ./bootstrap-prompt.md – Your first message.  
- ./codeskill-prompt.md – What the agent thinks with.  

That's it. No dependencies, no binaries—just prompts.

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
