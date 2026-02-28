# SeedClaw Bootstrap Prompt – v1.2 Self-Bootstrapping & Isolation Edition (2026-02-25)

Use this prompt **directly** in GitHub Copilot, Cursor, Claude Artifacts, Aider, Continue.dev, or any coding agent to generate the initial Go project.

**Instructions for the coding agent:**

You are generating the **initial seed binary** for SeedClaw – a self-hosting AI agent platform.  
Read and strictly follow **all** documents from the repo https://github.com/PixnBits/SeedClaw:

- PRD.md (MVP scope, bootstrap checklist, constraints)
- ARCHITECTURE.md (principles, sandbox table, threat model, bootstrap flow)
- skills/sdlc/coder.md (embed the **exact** text of the section "**Full prompt to paste into the running SeedClaw binary:**" — everything starting from "You are CodeSkill v1.1 ..." to the end — as a raw string constant)

**Core Goal (new in v1.2):**
Make the seed binary **self-bootstrapping and self-improving**:
- On `./seedclaw --bootstrap` it automatically generates + compiles + registers **CodeSkill** (the code-generation skill).
- It then immediately uses the new CodeSkill to generate a sophisticated **LLM invocation skill** (`llm_invoke`).
- This pulls **all LLM invocation logic out of the trusted seed binary** into sandboxed skills → the seed itself contains only a minimal stdlib HTTP client (used once during bootstrap).

**Strict Generation Rules:**
- **Dependencies (minimal):** ONLY stdlib + Docker client libs:
  - github.com/docker/docker/client (latest compatible)
  - github.com/docker/go-connections/nat (latest)
  - No go-openai, no other LLM libs in the seed binary.
- LLM calls in the seed: **only** the one-time bootstrap call, implemented with pure `net/http` + `encoding/json` (support Ollama and OpenAI-compatible endpoints via env vars).
- Embed the full CodeSkill prompt from `skills/sdlc/coder.md` as:
  ```go
  const codeSkillBootstrapPrompt = `You are CodeSkill v1.1 — ... (exact full text from skills/sdlc/coder.md)`
  ```
- CLI: support `--bootstrap` (perform full self-bootstrap) and `--start` (normal chat loop, default).
- If `--bootstrap` (or `--start` with empty registry), run the full self-bootstrap sequence described below.
- Docker compile step: `go vet ./... && go build -o /out/binary /src/main.go` (stdlib-only → no `go mod tidy` needed for skills in MVP).
- Sandbox for LLM skills (`codeskill`, `llm_invoke`): `NetworkMode: "bridge"` (or `"host"` for simple localhost Ollama access); all other skills default `network=none`.
- Skill execution: parse input as `SkillName: args`. Spawn fresh Docker container with the registered binary, pass args via stdin, expect stdout JSON `{result: string, error: string|null}`. If `result` unmarshals to a code-gen struct `{code, binary_name, hash, description}`, automatically compile/register the new skill.
- After bootstrap, the seed prints a clear success message and can continue into chat mode.

**Bootstrap Sequence the seed MUST implement:**
1. Use minimal stdlib LLM call with:
   - User message = `codeSkillBootstrapPrompt` + `\n\nInitial bootstrap request: Generate the CodeSkill skill binary now. The "code" must be a complete package-main Go program that:\n- Hardcodes the full CodeSkill prompt logic.\n- Reads request from os.Stdin.\n- Appends it as "User request: " + request.\n- Calls the LLM via stdlib net/http (same env config).\n- Returns exactly {"result": llm_content, "error": null}.\nOutput ONLY valid JSON.`
2. Parse JSON → compile/register as "codeskill".
3. Immediately invoke the new "codeskill" skill (via Docker) with request:  
   `add llm_invoke skill: a sophisticated reusable LLM invocation skill. Read JSON from stdin: {"base_url":string, "api_key":string, "model":string, "messages":[...], "temperature":float}. Use stdlib net/http to call /v1/chat/completions (support headers, retries, basic error handling). Return {"content":string, "full_response":object, "error":null}. binary_name=llm_invoke`
4. The CodeSkill binary will return the code-gen JSON in its `result` → seed auto-compiles/registers "llm_invoke".
5. Print: "✅ Self-bootstrap complete! CodeSkill and LLM invocation skill generated. LLM calls are now fully isolated in sandboxed skills. Run with --start to chat."

**Example: Minimal stdlib LLM call (include this pattern, adapt exactly):**
```go
type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
type ChatRequest struct {
    Model       string         `json:"model"`
    Messages    []ChatMessage  `json:"messages"`
    Temperature float64        `json:"temperature"`
}
type ChatChoice struct {
    Message struct {
        Content string `json:"content"`
    } `json:"message"`
}
type ChatResponse struct {
    Choices []ChatChoice `json:"choices"`
}

func minimalLLMCall(prompt string) (string, error) {
    baseURL := os.Getenv("LLM_BASE_URL")
    if baseURL == "" { baseURL = "http://localhost:11434/v1" }
    model := os.Getenv("LLM_MODEL")
    if model == "" { model = "llama3.2" } // or whatever default
    apiKey := os.Getenv("LLM_API_KEY")
    if apiKey == "" { apiKey = "ollama" }

    reqBody := ChatRequest{
        Model: model,
        Messages: []ChatMessage{{Role: "user", Content: prompt}},
        Temperature: 0.1,
    }
    body, _ := json.Marshal(reqBody)

    req, _ := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    if apiKey != "ollama" {
        req.Header.Set("Authorization", "Bearer "+apiKey)
    }

    client := &http.Client{Timeout: 120 * time.Second}
    resp, err := client.Do(req)
    // ... handle err, read body, unmarshal, return resp.Choices[0].Message.Content
}
```

**Example: Docker sandbox for LLM skills (allow network):**
```go
hostConfig := &container.HostConfig{
    Binds:          []string{fmt.Sprintf("%s:/src:ro", tmpDir)},
    AutoRemove:     true,
    ReadonlyRootfs: true,
    NetworkMode:    container.NetworkMode("bridge"), // or "host" for localhost Ollama
    CapDrop:        []string{"ALL"},
    Memory:         512 << 20,
    NanoCPUs:       1_000_000_000,
    SecurityOpt:    []string{"seccomp=unconfined"}, // tighten later
}
```

**Bootstrap Success Checklist (code must pass ALL):**
- [ ] Pure stdlib LLM for bootstrap only (no go-openai in seed)
- [ ] `--bootstrap` flag auto-generates + registers CodeSkill + llm_invoke
- [ ] CodeSkill prompt fully embedded and used
- [ ] LLM invocation pulled into sandboxed skills (llm_invoke is sophisticated, network-isolated)
- [ ] Skills compile/run with correct sandbox (network=bridge for LLM skills)
- [ ] Prefix parsing "SkillName: args" + auto code-gen detection
- [ ] Compiles cleanly with `go build -ldflags="-s -w"`

**Output format:** One fenced code block per file (`go.mod` first, then `seedclaw.go`, then `.env.example`).  
Add clear comments: `// See PRD.md section 3`, `// Self-bootstrap for isolation (v1.2)` etc.

This produces a seed binary that truly starts from almost nothing and immediately grows itself with better isolation. Happy bootstrapping — now the swarm can evolve without ever touching the trusted core again!
