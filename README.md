# omni

Omni is a supervisor for AI coding agents (Claude, Codex, Gemini). It manages agent sessions, hooks, and inter-agent messaging over a local PTY daemon and MCP transport.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Shaik-Sirajuddin/memory/main/install.sh | bash
```

Linux and WSL only (amd64 / arm64). Requires `sudo`.  
→ See [docs/quickstart.md](docs/quickstart.md) for what gets installed, upgrade instructions, and agent commands.

## Quick reference

```bash
omni agent list                          # list running sessions
omni agent resume <session-id>           # attach to a session
omni agent exec <session-id> -- <cmd>    # run a command inside a session
```

## Development

```bash
make docker-up       # start the dev container (requires .env.docker)
make docker-rebuild  # rebuild image after code changes
make docker-connect  # open a shell in the running container
```

→ See [development/](development/) for docker setup and `.env.docker.example`.
