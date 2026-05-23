# Docker Dev Environment

Run `omni` + `omni-server` inside an Ubuntu+systemd container, built from your local source.

## Prerequisites

- Docker + Docker Compose v2
- The project checked out locally

## First-Time Setup

```bash
make dev-preflight
```

This creates:
- `development/.env.docker` — copy of the example, fill in your API keys
- `development/local/` — agent MCP configs (`.claude.json`, `.codex/`, `.gemini/`)

Then edit `development/.env.docker` with your keys:

| Variable | Agent | Purpose |
|---|---|---|
| `ANTHROPIC_API_KEY` | claude | API key |
| `OPENAI_API_KEY` | codex | API key |
| `GEMINI_API_KEY` | gemini | API key |
| `ANTHROPIC_MODEL` | claude | Default model (e.g. `claude-sonnet-4-6`) |
| `CODEX_MODEL` | codex | Default model (e.g. `codex-mini-latest`) |
| `GEMINI_MODEL` | gemini | Default model (e.g. `gemini-2.5-pro`) |

## Start

```bash
make docker-up
```

On first start the container will:
1. Install binaries from the pre-built image
2. Write the systemd unit
3. Hand off to systemd — `omni@root.service` starts automatically

## Connect

```bash
make docker-connect
```

Opens an interactive login shell inside the container.

## Rebuild Image

Required when Go source or Dockerfile changes:

```bash
make docker-rebuild
```

Agent configs and DB persist across rebuilds — they are bind-mounted from `development/local/` and named volumes.

## Health Checks

```bash
# MCP streamable HTTP (agent clients)
curl -s http://127.0.0.1:18062/mcp

# Pure HTTP service (unix socket)
docker compose -f development/docker-compose.yaml exec ubuntu \
  curl -s --unix-socket /run/omni-root/service.sock http://unix/healthz

# systemd service status
docker compose -f development/docker-compose.yaml exec ubuntu \
  systemctl status omni@root --no-pager
```

## Service Logs

```bash
# all logs
docker compose -f development/docker-compose.yaml exec ubuntu journalctl -fu omni@root

# omni only (exclude tunnel_mcp)
docker compose -f development/docker-compose.yaml exec ubuntu \
  journalctl -fu omni@root | grep -v 'source=/build/tunnel_mcp'

# tunnel_mcp only
docker compose -f development/docker-compose.yaml exec ubuntu \
  journalctl -fu omni@root | grep 'source=/build/tunnel_mcp'
```

## Volumes and Persistence

| Source | Container path | Purpose |
|---|---|---|
| `omni-persist` (volume) | `/var/lib/omni-root` | ptydaemon DB, state |
| `omni-auth` (volume) | `/root/.config` | agent CLI auth tokens, omni config |
| `omni-data` (volume) | `/root/.local/share/memory` | agent DB, sandbox data |
| `development/local/.claude.json` | `/root/.claude.json` | claude MCP config |
| `development/local/.codex/` | `/root/.codex/` | codex MCP config |
| `development/local/.gemini/` | `/root/.gemini/` | gemini MCP config |

All data survives `make docker-down docker-up`. To fully reset:

```bash
docker compose -f development/docker-compose.yaml down -v   # removes named volumes
rm -rf development/local && make dev-preflight               # resets agent configs
```

## Stop

```bash
make docker-down
```
