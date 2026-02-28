# LLMSelectorSkill Prompt

You are generating **LLMSelectorSkill** — an intelligent router that decides which LLM wrapper skill should handle a given prompt/task.

## Purpose
- Receive incoming task messages via stdin (JSON envelope)
- Analyze prompt content and metadata
- Choose the best available LLM skill (OllamaSkill, GrokSkill, etc.)
- Forward the task to that skill via stdout (using message hub format)
- Return the response back through the hub

## Selection Heuristics (hard-code these rules)
- Contains "code", "golang", "compile", "generate skill" → prefer code-specialized model (e.g. deepseek-coder or similar)
- Contains "reason", "analyze", "explain", "research" → prefer general-reasoning model (e.g. llama3.1 70b or equivalent)
- Short/quick/simple → fastest local model (Ollama small)
- Mentions "x.ai", "grok", "real-time" → GrokSkill if available
- Default → OllamaSkill

## Strict Rules
1. Use only stdlib + basic json/time packages.
2. Discover available LLM skills by listening for their registration messages.
3. Accept config via env var: LLM_SELECTOR_PREFERENCES (JSON string, optional)
4. Forward full task payload unmodified, only change "to" field.
5. Include reasoning in a "metadata" field in the forwarded message.
6. Timeout forwarding after 90 seconds.

## Output JSON Format

```json
{
  "skill_name": "LLMSelectorSkill",
  "description": "Routes prompts to the most appropriate LLM wrapper skill",
  "prompt_template": "You are LLMSelectorSkill. Choose best LLM based on heuristics and forward the task.",
  "go_package": "main",
  "source_code": "... complete Go code ...",
  "binary_name": "llmselector",
  "build_flags": ["-trimpath", "-ldflags=-s -w"],
  "tests_included": true,
  "test_command": "go test -v ./..."
}
```

Generate complete single-file implementation + tests.
