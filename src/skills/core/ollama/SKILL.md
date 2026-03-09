# Ollama Skill

This skill manages a local Ollama instance for running Large Language Models. It provides a containerized Ollama server that can be used by other skills for LLM inference.

## Capabilities

- Runs Ollama server in a secure container
- Manages model downloads and serving
- Provides REST API for model inference
- Registers with message-hub for coordination

## Environment Variables

- None required (uses default Ollama configuration)

## Message Format

### Incoming Messages
```json
{
  "from": "sender",
  "to": "ollama",
  "content": {
    "action": "pull",
    "model": "llama3.2:latest"
  }
}
```

Supported actions:
- `pull`: Download and install a model
- `serve`: Start serving (default on startup)
- `list`: List available models

### Outgoing Messages
```json
{
  "from": "ollama",
  "to": "sender",
  "content": {
    "status": "success",
    "models": ["llama3.2:latest", "qwen2.5-coder:32b"]
  }
}
```

## API Endpoint

The Ollama server is available at `http://ollama:11434` within the Docker network.

## Models

- `llama3.2:latest`: General purpose model for llm-caller
- `qwen2.5-coder:32b`: Code generation model for coder skill