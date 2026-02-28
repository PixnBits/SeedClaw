# SeedClaw – Self-Hosting Agent Seed

**Zero-code, zero-trust bootstrap for your own local AI agent swarm.**  
Run one tiny Go binary → feed it prompts → watch it write, compile, and register new skills. Then grow forever.

## Quick Start (POSIX-friendly – prefer piping!)

### 1. Generate the seed binary (one-time)

```bash
git clone https://github.com/PixnBits/SeedClaw
cd SeedClaw
cat bootstrap-prompt.md | pbcopy   # or xclip -sel clip < bootstrap-prompt.md
```

Paste into your coding agent → generate go.mod + seedclaw.go → build:

```bash
go mod tidy
go build -o seedclaw .
```

### 2. Run & bootstrap

```bash
./seedclaw
# or with env overrides: OLLAMA_MODEL=llama3.1:70b ./seedclaw
```

### 3. Bootstrap foundational skills (pipe files!)

The core binary reads stdin until EOF. Piping markdown is safest:

```bash
# 1. Core coder (required first)
cat skills/sdlc/coder.md | ./seedclaw

# 2. Message hub (enables inter-skill comms)
cat skills/comms/messagehub.md | ./seedclaw

# 3. LLM selector + local wrapper
cat skills/llm/selector.md | ./seedclaw
cat skills/llm/ollama-wrapper.md | ./seedclaw

# 4. Core reasoning & resilience
cat skills/core/memory-reflection.md   | ./seedclaw   # archives generations, pre-git bridge
cat skills/planning/planner.md         | ./seedclaw
cat skills/verification/critic.md      | ./seedclaw
cat skills/recovery/retry-orchestrator.md | ./seedclaw
cat skills/evolution/self-mod.md       | ./seedclaw

# 5. Version control (commits pre-git skills from memory)
# (add skills/sdlc/git.md when ready)
```

Watch stdout for delegation messages (e.g., "Delegating LLM calls to OllamaSkill").

### 4. Grow the swarm

```bash
# Example routed command
echo 'codeskill: generate a skill that lists files in sandbox' | ./seedclaw
```

## Delegation Model (how the core shrinks)

- Initial: core handles LLM calls & generation.
- After registration: core delegates to skills (e.g., LLMSelectorSkill, CodeSkill).
- Core stays immutable & minimal — only routes and registers.

## Recommended Order Recap

1. CodeSkill
2. MessageHubSkill
3. LLMSelectorSkill + OllamaSkill
4. MemoryReflectionSkill (stores generations)
5. PlannerSkill
6. CriticSkill
7. RetryOrchestratorSkill
8. SelfModSkill
9. GitSkill (bulk commit from memory)

## Security Notes

- Core binary: immutable, no self-mod, no disk writes.
- All generation/execution: strict Docker sandboxes.
- Secrets: env vars only, injected at runtime.

MIT license. Bootstrap responsibly.