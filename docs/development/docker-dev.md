# Docker Dev Environment

Run `omni` + `omni-server` inside an Ubuntu+systemd container, built from your local source.

## Prerequisites

- Docker + Docker Compose v2
- The project checked out locally

## Start

```bash
cd <repo-root>
docker compose -f development/docker-compose.yaml up --build
```

On first start the container will:
1. Install agent CLIs (`claude`, `codex`, `gemini`) via npm
2. Build `omni` and `omni-server` from mounted source
3. Install binaries + write the systemd unit
4. Hand off to systemd — `omni@root.service` starts automatically

## Agent API Keys

Copy the example env file and fill in your keys:

```bash
cp development/.env.docker.example development/.env.docker
# edit development/.env.docker with your keys
```

| Variable | Agent | Purpose |
|----------|-------|---------|
| `ANTHROPIC_API_KEY` | claude | API key |
| `OPENAI_API_KEY` | codex | API key |
| `GEMINI_API_KEY` | gemini | API key |
| `ANTHROPIC_MODEL` | claude | Default model (e.g. `claude-sonnet-4-6`) |
| `CODEX_MODEL` | codex | Default model (e.g. `codex-mini-latest`) |
| `GEMINI_MODEL` | gemini | Default model (e.g. `gemini-2.5-pro`) |

The container reads `.env.docker` on startup — no interactive login required.

## Subsequent Starts

```bash
docker compose -f development/docker-compose.yaml up
```

Source is live-mounted — rebuild inside the container to pick up changes:

```bash
docker compose -f development/docker-compose.yaml exec ubuntu bash /workspace/development/build.sh
systemctl restart omni@root
```

Or restart the whole container:

```bash
docker compose -f development/docker-compose.yaml restart
```

## Health Checks

Verify all three services are up after start or rebuild:

**Unix socket (tunnel-mcp — default transport):**
```bash
curl -s --unix-socket /run/omni-${USER}/mcp.sock http://unix/healthz
# → {"status":"ok","service":"tunnel-mcp","version":"..."}
```

**TCP (tunnel-mcp — only when `AXO_LINK_MCP_TRANSPORT=http` or `streamable_http`):**
```bash
curl -s http://127.0.0.1:18061/healthz
```

**systemd service status:**
```bash
docker compose -f development/docker-compose.yaml exec ubuntu \
  systemctl status omni@root --no-pager
```

## Service Logs

```bash
docker compose -f development/docker-compose.yaml exec ubuntu \
  journalctl -u omni@root -f
```

## Environment Overrides

Create `development/.env.docker` to override defaults:

```bash
DEBUG=1          # enables slog debug logging (DEV=1 inside the service)
VERSION=dev      # override build version string
```

## Volumes

| Volume | Mount | Purpose |
|--------|-------|---------|
| `omni-persist` | `/var/lib/omni-root` | ptydaemon DB, state |
| `omni-auth` | `/root/.config` | agent CLI auth tokens |

To reset auth (force re-login):

```bash
docker volume rm cli_omni-auth
```

## Stop

```bash
docker compose -f development/docker-compose.yaml down
```
