# SeedClaw Bootstrap Prompt – v1.3 (delegation-focused, minimal core)

Feed this prompt directly into a coding agent (Cursor, Claude, Aider, Continue.dev, etc.) to generate the initial `seedclaw` binary.

**Instructions for the coding agent:**

Generate the **seed binary** for SeedClaw — paranoid, local-first, self-extending agent system.

Strictly follow:
- https://github.com/PixnBits/SeedClaw/PRD.md
- https://github.com/PixnBits/SeedClaw/ARCHITECTURE.md

**Core Philosophy (must obey):**
- The binary is immutable after generation — never self-modifies on disk.
- Keep attack surface tiny: stdin/stdout only externally; Docker SDK internally; minimal LLM client for bootstrap only.
- Delegate everything possible once skills are registered (registry overrides core behavior).

**Minimal Responsibilities of the Seed Binary:**
1. Read multi-line input from os.Stdin until EOF (bufio.Scanner or io.ReadAll).
2. If input starts with "skillname:", route to the registered binary for that skill via sandboxed subprocess (pipe stdin/stdout, capture output).
3. If no matching skill (or bootstrap mode), forward full input to local LLM → parse JSON → sandbox compile/test/register.
4. In-memory registry: map[string]Skill {Name, PromptTemplate, BinaryPath, Hash, DelegateFunc}.
5. On registration success: update registry, print "SUCCESS: Skill 'X' registered\nPath: ...\nHash: ...\nDelegating future calls to /tmp/skills/X"
6. LLM calls: use minimal http.Client to Ollama (default localhost:11434) **only during bootstrap**.  
Default model: `qwen2.5-coder:32b` (or user env OLLAMA_MODEL).  
Strongly prefer code-specialized models like qwen2.5-coder for generation tasks.  
Once OllamaSkill or LLMSelectorSkill registers, delegate all future LLM calls to it (update a global llmCaller func).
7. Retry logic: up to 3× on parse/compile/vet failure (append error to next LLM prompt).
8. Verbose stdout logging at every step.
9. Security: context timeouts, Network=none, ReadonlyRootfs=true, CapDrop=ALL, user=nobody in containers.

**JSON Expected from LLM (strict parse):**

```json
{
  "skill_name": "CodeSkill",
  "description": "...",
  "prompt_template": "...",
  "go_package": "main",
  "source_code": "...full code...",
  "binary_name": "codeskill",
  "build_flags": ["-trimpath", "-ldflags=-s -w"],
  "tests_included": true,
  "test_command": "go test -v ./..."
}
```

**Delegation Rules (critical):**
- After any skill registers, check if it can take over core functions (e.g., if skill_name == "OllamaSkill", set llmCaller = func(prompt) { return execSandboxBinary(skill.BinaryPath, prompt) }).
- Core never writes files or persists state beyond in-memory registry.
- Use github.com/docker/docker/client (resolve version automatically).
- Dependencies: stdlib + go-openai (for initial LLM) + docker/client.

**Extra Guidance:**
- Handle large piped inputs (skills/*.md files) robustly.
- Include --start / --help flags.
- Add .env.example for OLLAMA_URL, OLLAMA_MODEL, etc.
- Output: go.mod + main.go (seedclaw.go) content.
- Recommend users pull `ollama pull qwen2.5-coder:32b` (or q5_K_M quantized variant if VRAM-limited) for best code generation quality in CodeSkill loops.
- Include fallback to llama3.2:latest if model not found.

Generate the complete project now.