# Ollama Skill v2.1.3

Manages local Ollama instance for model serving and pulls. Containerized with strict isolation.

## Capabilities
- Serve models locally (http://ollama:11434)
- Pull models on demand
- List available models

## Network Policy (v2.1 Mandatory)
```json
{
  "name": "ollama",
  "required_mounts": ["ollama/models:rw"],
  "network_policy": {
    "outbound": "allow_list",
    "domains": ["registry.ollama.ai", "ollama.com"],
    "ports": [443],
    "network_mode": "seedclaw-net"
  },
  "network_needed": true
}
```
Outbound ONLY for model registry pulls. Blocked otherwise.

## Resource Exception (audited, ollama-only)
`mem_limit: 16g` + `shm_size: 1g` (required for 30 B models on consumer hardware; all other skills remain at 512 m).

## Recommended Default Model for RTX 3060-class hardware
`nemotron-3-nano:30b-q4_K_M` (NVIDIA)

## Required Mounts
["ollama/models:rw"] (for model storage; managed by seedclaw). No other shared/ access.

## Default Container Runtime Profile
Compliant except the explicit mem_limit exception above: read_only: true (except models tmpfs), cap_drop: [ALL], mem_limit overridden for ollama only, network: seedclaw-net.

## Communication
Exclusively via message-hub. Other skills call ollama only through hub. Internal serving on Docker network only.

## Message Format
**Incoming:**
```json
{
  "from": "sender",
  "to": "ollama",
  "content": {
    "action": "pull",
    "model": "nemotron-3-nano:30b-q4_K_M"
  }
}
```

## Implementation Wiring
You MUST read and follow CORE_IMPLEMENTATION_PATTERNS.md verbatim before generating any code.
Copy the exact skeletons where applicable.
At the end of generation, output the filled compliance checklist from that file.
