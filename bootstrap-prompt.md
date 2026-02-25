# SeedClaw Bootstrap Prompt – v1.1 (2026-02-25)

Use this prompt **directly in GitHub Copilot, Cursor, Claude Artifacts, Aider, Continue.dev, or similar coding agent** to initialize the Go project and generate `seedclaw.go`.

**Instructions for the coding agent:**

You are generating the **initial seed binary** for SeedClaw – a self-hosting AI agent platform.  
Read and strictly follow these documents from the repo https://github.com/PixnBits/SeedClaw:

- PRD.md (Product Requirements Document) – defines mission, MVP scope, constraints, and bootstrap success checklist.
- ARCHITECTURE.md – details principles, components, sandbox model, threat model, and bootstrap flow diagram.

**Task:**
Generate a complete, minimal, compilable Go project that implements the **seed binary** as described.

**Strict Generation Rules (follow exactly):**
- Use ONLY these libraries (add to go.mod):
  - github.com/sashabaranov/go-openai v1.35.0 (for Ollama compatibility — point BaseURL to http://localhost:11434/v1)
  - github.com/docker/docker/client v27.5.0+incompatible
  - github.com/docker/go-connections/nat
  - stdlib only otherwise
- LLM call MUST be real HTTP (no stubs, no fake returns).
- Docker compile step MUST include: `go vet ./...` + `go build`
- Test step MUST: compile → then run the binary in a SECOND container (network=none, readonly) and capture output.
- Include full go.mod with exact versions.
- Output format: one code block per file (go.mod first, then seedclaw.go, then .env.example)
- Add comments in code: "// See PRD.md section 3" and "// See ARCHITECTURE.md sandbox table"

**Example: Real Ollama-compatible LLM call (adapt this exactly)**  
```go
import "github.com/sashabaranov/go-openai"

client := openai.NewClientWithConfig(openai.ClientConfig{
    BaseURL: "http://localhost:11434/v1", // or env var
    APIKey:  "ollama",                    // dummy key for Ollama
})
resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
    Model:    llmModel, // from env
    Messages: []openai.ChatCompletionMessage{
        {Role: openai.ChatMessageRoleUser, Content: fullPrompt},
    },
    Temperature: 0.1, // low for deterministic code gen
})
if err != nil { /* handle */ }
content := resp.Choices[0].Message.Content
// Then parse JSON from content
```

**Example: Secure Docker compile container (adapt flags)**
```go
resp, err := cli.ContainerCreate(ctx, &container.Config{
    Image: "golang:1.23-alpine",
    Cmd:   []string{"sh", "-c", "go vet ./... && go build -o /out/binary /src/main.go"},
}, &container.HostConfig{
    Binds:          []string{fmt.Sprintf("%s:/src:ro", tmpDir)},
    AutoRemove:     true,
    ReadonlyRootfs: true,
    NetworkMode:    "none",
    CapDrop:        []string{"ALL"},
    Memory:         256 << 20,     // 256MB
    NanoCPUs:       500_000_000,   // 0.5 CPU
    SecurityOpt:    []string{"seccomp=unconfined"}, // replace with profile later
}, nil, nil, "")
```

**Use these as patterns** — do not copy verbatim; integrate into your full implementation.

**Bootstrap Success Checklist** (code must satisfy all):
* [ ] Real LLM HTTP call works with Ollama
* [ ] Docker compile + go vet succeeds
* [ ] Binary runs in second sandbox and returns test output
* [ ] Skills register and can be listed
* [ ] Security flags exactly as in ARCHITECTURE.md
