# Coder Skill

This skill generates new skills for the SeedClaw platform. It uses advanced code generation models to create complete skill implementations including Go source code, Dockerfiles, and registration metadata.

## Capabilities

- Reads SKILL.md prompt templates
- Generates complete skill packages
- Uses qwen2.5-coder:32b for high-quality code generation
- Produces sandbox-ready containers

## Environment Variables

- None required

## Message Format

### Incoming Messages
```json
{
  "from": "sender",
  "to": "coder",
  "content": {
    "action": "generate_skill",
    "skill_name": "example-skill",
    "prompt": "Create a skill that..."
  }
}
```

### Outgoing Messages
```json
{
  "from": "coder",
  "to": "sender",
  "content": {
    "status": "success",
    "skill": {
      "name": "example-skill",
      "code": "...",
      "dockerfile": "...",
      "metadata": {...}
    }
  }
}
```

## Model

Uses `qwen2.5-coder:32b` for code generation tasks.