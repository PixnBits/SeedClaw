# SeedClaw Architecture

SeedClaw is a **minimal, paranoid, local-first, self-extending agent system** designed to bootstrap itself from a tiny Go binary into a capable swarm of sandboxed skills — without ever committing agent code to the repository.

## Core Principles

1. **Zero trust & paranoia first**  
   - Nothing is trusted by default — not generated code, not LLM output, not even the seed binary after initial run.  
   - All execution happens in strict, ephemeral sandboxes.  
   - No persistent storage of any kind (registry is in-memory only).  
   - No network access unless explicitly requested and sandboxed per skill.

2. **Tiny immutable seed**  
   - The seed binary (~20–80 KB) is the only compiled artifact in the repo (generated via external agent).  
   - It never changes after bootstrap; all growth happens via generated & registered skills.

3. **Prompt-driven emergence**  
   - New capabilities are created by pasting natural-language requests to already-registered skills (starting with CodeSkill).  
   - No hand-written agent logic beyond the seed.

4. **Local-first & offline-capable**  
   - Default LLM: Ollama running on localhost.  
   - Optional remote LLMs via per-skill wrappers with runtime-injected secrets (never hard-coded).

5. **Sandbox evolution path** (increasing isolation)

   | Stage       | Executor                  | Isolation Level          | Notes                              |
   |-------------|---------------------------|--------------------------|------------------------------------|
   | Current     | Docker (via Go SDK)       | High                     | Network=none, readonly rootfs, no caps |
   | Next        | gVisor / runsc            | Very High                | Kernel-level sandbox               |
   | Future      | Firecracker microVM       | Extremely High           | Full VM boundary                   |
   | Long-term   | WASM + wasi / wasmtime    | High + lightweight       | No container runtime needed        |

## Bootstrap Flow & End State

The seed binary starts in a minimal state and reaches a **self-sufficient end state** through this loop:

1. User runs `./seedclaw` (or equivalent) and pastes the bootstrap prompt (from `bootstrap-prompt.md`).
2. Seed calls local LLM → receives JSON → writes temp source → spawns **Container 1** (golang:1.22-alpine) to compile + vet.
3. On success: spawns **Container 2** (minimal/scratch) to test + hash binary.
4. On success: registers **CodeSkill** in in-memory map.
5. Seed prints success message and awaits further stdin commands.

**Desired end state after bootstrap + a few iterations**:
- CodeSkill is registered and functional.
- The agent can reliably respond to "codeskill: generate a skill that …" requests.
- At least 4–8 foundational skills have been generated and registered without external intervention:
  - MessageHubSkill (pub/sub router via JSON-lines stdin/stdout)
  - LLMSelectorSkill (routes prompts to best LLM based on task type/metadata)
  - One or more per-LLM wrapper skills (OllamaSkill, GrokSkill, etc.) with secret isolation
  - MemoryReflectionSkill (memory + reflection; pre-git archive)
  - PlannerSkill (task decomposition)
  - CriticSkill (verification/critique)
  - RetryOrchestratorSkill (failure recovery)
  - SelfModSkill (meta-evolution)
  - GitSkill (local VCS; bulk commits pre-git archive from memory)
- Skills can compose via structured messages through the hub.
- Every generation attempt passes:
  - Compilation
  - Static analysis (go vet minimum)
  - Basic runtime test
  - Security invariants (no unsafe patterns, no leaked secrets)
- Total bootstrap-to-useful-swarm time: ideally < 10 minutes with a capable local LLM.

If any step fails repeatedly, the seed provides clear stdout diagnostics and allows retry via re-pasting a refined prompt.

## Pre-Git Archiving Flow
- CodeSkill generates skill → sends "store" to MemoryReflectionSkill with full metadata (source, prompt, hash, etc.)
- MemoryReflectionSkill archives in memory (opt-in file append)
- After GitSkill registers: SelfModSkill or PlannerSkill triggers batch retrieve from memory → sends to GitSkill for bulk commit

## Core Components

- **Seed Binary**  
  - stdin → LLM → JSON → Docker SDK → Container 1 (build/vet) → Container 2 (test/hash) → register or retry  
  - Uses official `github.com/docker/docker/client` (no shell exec)  
  - Verbose progress logging on stdout  
  - In-memory skill registry: `map[string]Skill` (name → prompt template + binary path + hash + metadata)

- **Skill Registry**  
  - Ephemeral (lost on restart — intentional)  
  - Skill = {Name, PromptTemplate, BinaryPath, Hash, RegisteredAt}

- **Inter-Skill Messaging Standard**  
  - JSON lines over stdin/stdout  
  - Mandatory envelope: `{from, to, type, payload, id, timestamp}`  
  - Enables loose coupling and future orchestration (hub routes, selector dispatches)

- **LLM Management**  
  - Secrets never in code or prompts  
  - Injected via environment variables at container runtime  
  - Future LLMSelectorSkill decides routing (code-gen → specialized model, reasoning → another, etc.)

- **Resilience Features**  
  - Up to 3 LLM retries per generation with error feedback appended  
  - Clear stdout checkpoints at every stage  
  - Graceful degradation on timeouts / parse failures  
  - No partial registrations — all or nothing

## Security invariants (checked at every generation)

- No network unless explicitly allowed per skill
- Readonly rootfs + no capabilities
- User nobody / UID 65534
- Context timeouts everywhere
- Static analysis blocks unsafe.{Pointer,Slice}, syscall, dangerous exec patterns
- Secrets only via env — never printed/logged

## Evolution & Non-Goals

Non-goals (by design):
- Persistent memory / long-term state across runs
- Multi-user / authentication
- Cloud coordination
- GUI / web interface
- Package management beyond go mod in containers

Future extensions should remain emergent — generated via CodeSkill or successor skills — never hand-written in the repo.

This architecture enables a single tiny binary to grow, under strict constraints, into a swarm of sandboxed, composable agents — all driven by natural language and local compute.
