# SeedClaw Bootstrap Prompt – v2.0 – Thin Orchestrator + Core Skills (2026-03)

Use this prompt directly in GitHub Copilot, Cursor, Claude Artifacts, Aider, Continue.dev, Grok web chat, or similar coding agent.

Context documents (read from this repo):
- PRD.md
- ARCHITECTURE.md (version 1.2 – Thin Host + Controlled IPC)

## Task
Generate a minimal, production-ready starting point for SeedClaw consisting of **exactly** the following:

1. The thin host orchestrator: `seedclaw.go`
   - Written in Go 1.21+.
   - Uses `github.com/docker/docker/client` and related Docker types.
   - Exposes a narrow Unix socket IPC server on `~/.seedclaw/ipc.sock` (0600 perms), line-delimited JSON.
   - Implements IPC actions: `start_skill`, `stop_skill`, `restart_skill`, `get_logs`, `get_status`.
   - On startup, parses four markdown skill definitions and starts one container per skill.
   - Provides chat input via stdin (and optional Telegram if `TELEGRAM_BOT_TOKEN` is set).
   - Does **not** call any LLMs or compile code itself.

2. Four core skill definition files in `skills/` directory:
   - `skills/core/messagehub.md`
   - `skills/core/llmcaller.md`
   - `skills/sdlc/coder.md`
   - `skills/sdlc/skill-builder.md`
   Each file must include:
   - A clear description of the skill role.
   - Input and output JSON formats.
   - Security rules.
   - A **Docker run spec** block with **exact** fields:
     - `Image:`
       - `seedclaw-messagehub:latest`
       - `seedclaw-llmcaller:latest`
       - `seedclaw-coder:latest`
       - `seedclaw-skill-builder:latest`
     - `Mount:` including `"$HOME/.seedclaw/ipc.sock:/ipc.sock:ro"` for IPC, plus `/tmp:/tmp:rw` for coder/skill-builder.
     - `Network:`
       - `none` for `messagehub` and `skill-builder`.
       - `host` for `llmcaller` and `coder` so they can reach local LLMs.
     - `Command:`
       - `/app/messagehub`
       - `/app/llmcaller`
       - `/app/coder`
       - `/app/skill-builder`.

## Orchestrator behavior (seedclaw.go)

Your `seedclaw.go` must:

1. Define types:
   - `SkillConfig` with fields: `Name`, `Image`, `Mounts []string`, `Cmd []string`, `Network string`.
   - `IPCRequest` with fields: `Action`, `Name`, `Image`, `Env map[string]string`, `Mounts []string`, `Cmd []string`, `Network string`, `Lines int`.

2. Initialize Docker client once at startup using `client.NewClientWithOpts(client.FromEnv)` and store it in a package-level `cli *client.Client`.

3. Set `ipcSock := filepath.Join(os.Getenv("HOME"), ".seedclaw", "ipc.sock")` and:
   - `os.MkdirAll(filepath.Dir(ipcSock), 0700)`
   - `os.Remove(ipcSock)` before listening.

4. Start a Unix socket IPC server in a goroutine:
   - `net.Listen("unix", ipcSock)`
   - `os.Chmod(ipcSock, 0600)`
   - For each connection, read line-delimited JSON, unmarshal into `IPCRequest`, and handle only the 5 allowed actions.

5. Implement `startCoreSkills()` to:
   - Hardcode the skill names: `messagehub`, `llmcaller`, `coder`, `skill-builder`.
   - Map skill name → folder: `messagehub`/`llmcaller` in `skills/core`, `coder`/`skill-builder` in `skills/sdlc`.
   - For each, call `parseMD` on the appropriate markdown file and then `startSkill` with the parsed config.

6. Implement `parseMD(path string) SkillConfig` to:
   - Open the markdown file.
   - Scan line by line until it finds the `**Docker run spec**` section.
   - Within that section, extract:
     - `- Image: ...` → `config.Image` (trimmed).
     - `- Mount: ...` → expand `$HOME` to `os.Getenv("HOME")`, then split on `", "` into `config.Mounts`.
     - `- Command: ...` → split on spaces into `config.Cmd`.
     - `- Network: ...` → first whitespace-separated token into `config.Network`.

7. Implement `startSkill(name, image string, env map[string]string, mounts []string, cmd []string, network string) string` to:
   - Build `envSlice []string` from the map.
   - Convert `mounts []string` of the form `"/host/path:/container/path[:ro]"` into `[]mount.Mount` with `Type: mount.TypeBind` and `ReadOnly` based on the third field.
   - Create a `container.Config` with `Image`, `Env`, `Cmd`, and `OpenStdin: true`.
   - Create a `container.HostConfig` with the mounts and `NetworkMode: container.NetworkMode(network)`.
   - Call `cli.ContainerCreate` with container name equal to the skill name.
   - Start the container with `cli.ContainerStart`.
   - Launch `readLogs(name)` in a goroutine.
   - Return a small JSON string `{"status":"started"}` or `{"error":"..."}` on failure.

8. Implement `readLogs(name string)` to:
   - Call `cli.ContainerLogs` with `container.LogsOptions{ShowStdout:true, ShowStderr:true, Follow:true}`.
   - Scan logs line by line, expecting each line to be a JSON message.
   - If `to == "user"` and `payload.text` is present, print the text to stdout (this is the user-visible chat output).
   - If `to` is any other non-empty string, call `sendToSkill(to, msg)` to forward the message.

9. Implement `stdinChat()` to:
   - Read from stdin line by line.
   - Wrap each line into a JSON message:
     - `{ "from": "user", "to": "messagehub", "type": "text", "payload": {"text": <line>} }`
   - Call `sendToSkill("messagehub", message)`.

10. Implement `sendToSkill(name string, msg interface{})` to:
    - Marshal `msg` to JSON.
    - Run `docker exec -i <name> sh -c "cat"` with the JSON followed by `"\n"` piped to stdin.
    - Ignore the command output.

11. Optionally implement `telegramChat(token string)` using `tgbotapi` to forward Telegram messages into the same `messagehub` entrypoint.

12. Main function must:
    - Initialize `cli`.
    - Set up and start the IPC server.
    - Call `startCoreSkills()`.
    - Start `stdinChat()` (and `telegramChat` if token is set) in goroutines.
    - Block forever with `select {}`.

## Output format

Provide complete files as fenced code blocks:
- `go.mod` (Go 1.21+, with Docker client and Telegram deps as used in `seedclaw.go`).
- `seedclaw.go` implementing the behavior specified above.
- `skills/core/messagehub.md`
- `skills/core/llmcaller.md`
- `skills/sdlc/coder.md`
- `skills/sdlc/skill-builder.md`.

Do **not** generate any other files in this prompt. Output only the files listed above.
