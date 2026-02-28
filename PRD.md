# SeedClaw Product Requirements Document (PRD)

SeedClaw is a **tiny, paranoid, local-first, self-bootstrapping agent system** that starts from a single small Go binary and grows — entirely through natural language prompts — into a swarm of sandboxed, composable skills.

## 1. Core Product Goal

A user with a local LLM (e.g. Ollama) should be able to:
1. Run a ~20–80 KB seed binary.
2. Paste one bootstrap prompt.
3. Within minutes reach a **self-sufficient coding agent** capable of reliably generating, vetting, testing, and registering new skills.
4. Iteratively grow that agent into a small swarm of interoperable skills — all without writing or committing any agent code.

## 2. Non-Goals (Explicit Exclusions)

- Persistent memory or long-term state across runs
- Multi-user support or authentication
- Cloud coordination, APIs, or remote execution
- GUI, web UI, or rich frontend
- Package managers, dependency graphs, or versioned skill releases
- Hand-written agent logic beyond the initial seed

## 3. Key Requirements

### 3.1 Seed Binary
- Single statically-linked Go binary
- Dependencies limited to stdlib + official Docker Go client (`github.com/docker/docker/client`)
- Accepts commands via stdin, emits feedback & results via stdout
- Uses programmatic Docker SDK (no shell `docker` exec)
- Enforces strict sandbox defaults on every container:
  - Network: none
  - Readonly rootfs
  - Drop all capabilities
  - Run as nobody / UID 65534
  - 30–60s timeouts
- Verbose, structured progress messages on stdout

### 3.2 Bootstrap Success Criteria
The system reaches “minimal viable agent” when:
- CodeSkill is registered and responds correctly to "codeskill: …" commands
- Generation loop succeeds ≥ 80% of the time on first or second LLM try
- Each new skill passes: compile → vet → basic test → hash → register
- Total time from `./seedclaw` to usable CodeSkill: ideally < 5 min

### 3.3 Foundational Skills (Bootstrapping Targets)
These should be generatable in the first 5–15 minutes after CodeSkill is live:

| Priority | Skill Name            | Purpose                                                                 | Dependencies / Notes                              |
|----------|-----------------------|-------------------------------------------------------------------------|---------------------------------------------------|
| 1        | CodeSkill             | Generates, compiles, vets, tests, registers new skills                  | Must exist first (bootstrap target)               |
| 2        | MessageHubSkill       | Pub/sub router using JSON-lines stdin/stdout; enables skill composition | Mandatory inter-skill messaging envelope          |
| 3        | LLMSelectorSkill      | Routes prompts to best LLM based on task type, model traits, heuristics | Uses message hub; reads env/config for available LLMs |
| 4        | OllamaSkill (wrapper) | Direct localhost Ollama caller; secrets via env only                    | Template for per-LLM wrappers                     |
| 5–N      | GrokSkill / OtherLLMSkill | Similar wrappers for remote APIs (keys via env)                     | Secret isolation enforced                         |
| 5        | MemoryReflectionSkill | Memory storage/retrieval + reflection; pre-git skill archive           | Stores generations for later git handoff          |
| 6        | PlannerSkill          | Task decomposition, planning, re-planning                              | Unlocks complex multi-step goals                  |
| 7        | CriticSkill           | Output verification, critique, quality scoring                         | Boosts reliability via self-critique              |
| 8        | RetryOrchestratorSkill| Failure monitoring, retries, refinements                               | Adds resilience to the swarm                      |
| 9        | SelfModSkill          | Meta-evolution: propose prompt/skill improvements                      | Enables recursive self-improvement                |
| 10       | GitSkill              | Local git for tracking generated skills; bulk commit from memory       | Commits pre-git archive; enables SDLC             |
| Bonus    | FileSkill, etc.       | Basic local utilities in strict sandboxes                              | Generated after orchestration basics are solid    |

### 3.4 Skill Generation & Validation Requirements
Every generated skill must:
- Be single-file Go (when reasonable)
- Use Go 1.22+ stdlib preferentially
- Include `_test.go` file with at least basic tests whenever logic is non-trivial
- Follow inter-skill messaging standard (JSON envelope over stdin/stdout)
- Never hard-code secrets — accept only via environment variables
- Pass static analysis (go vet minimum; golangci-lint preferred if available in sandbox)
- Be hashed (SHA256) and registered only on full success

### 3.5 Testing Strategy
- CodeSkill must generate unit tests alongside production code when possible
- Seed runs `go test` (or custom test command) in isolated container before registration
- Tests use only standard `testing` package
- Failure in tests → full retry loop with error feedback to LLM

### 3.6 User Success Checklist (communicated in README & stdout)
1. Run seed binary
2. Paste bootstrap prompt → see "CodeSkill registered"
3. Test: `codeskill: generate a skill that acts as a message hub`
4. Test: `codeskill: create an LLM selector skill`
5. Test: `codeskill: generate Ollama wrapper skill`
6. Continue through MemoryReflectionSkill → GitSkill
7. Verify chaining: send message through hub → selector → OllamaSkill
8. Iterate: ask for more utilities

### 3.7 Success Metrics (Observables)
- Bootstrap to CodeSkill: < 5 min, < 3 LLM calls
- First additional skill (e.g. MessageHub): < 3 min after CodeSkill live
- End-to-end skill generation success rate: ≥ 80% on first or second attempt
- No security violations logged (unsafe patterns, network leaks, secret exposure)
- Swarm of 5+ skills achievable in < 30 min of interactive prompting
- Pre-git to git handoff: bulk commit succeeds after GitSkill registration

### 3.8 Guiding Principles Recap
- Emergent growth via natural language only
- Paranoia > convenience
- Local compute only (Ollama default)
- Ephemeral everything
- Sandbox first, always

This PRD defines the minimal viable path from tiny seed → useful agent swarm. All future capabilities should be generated via CodeSkill or its successors — never added directly to the repository.
