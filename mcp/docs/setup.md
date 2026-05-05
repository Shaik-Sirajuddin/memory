# MCP Setup

Run all commands from the `mcp/` directory.

## Requirements

- Go 1.22 or newer
- A writable Go build cache

The Makefile defaults `GOCACHE` to `/tmp/go-build-mcp` so it works in restricted
workspaces where the normal home cache may be read-only.

## Build

```sh
make build
```

The binary is written to `.bin/mcp`.

## Run In Foreground

```sh
make run
```

`make run` builds `.bin/mcp` and runs it as an attached foreground process.
Stop it with `Ctrl-C`.

Useful overrides:

```sh
make run ADDR=127.0.0.1:8100 INTERVAL=30s
DEV=1 make run
```

## Run stdio Transport

```sh
make run-stdio
```

The stdio transport reads newline-delimited JSON-RPC from stdin and writes
newline-delimited JSON-RPC to stdout. When `-interval` is omitted, stdio sends a
scheduled `notifications/event` message every `10s` with this prompt:

```text
Run this example command and return the output: cat docs/setup.md
```

Quick initialize check:

```sh
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize"}' | make run-stdio
```

Direct Claude-style command:

```sh
.bin/mcp -transport stdio
```

Schedule check:

```sh
timeout 12s bash -lc '.bin/mcp -transport stdio < <(sleep 11)'
```

## Run In Background

```sh
make start
make status
make stop
```

Defaults:

- address: `:8100`
- interval: `1m`
- pid file: `.mcp.pid`
- log file: `.mcp.log`

Use a specific address when the default port is busy:

```sh
make start ADDR=127.0.0.1:18090
make stop
```

## Test

```sh
make test
```

This runs:

```sh
GOCACHE=/tmp/go-build-mcp go test -tags unit ./...
```

## Logs

Each server run writes structured logs to a new file:

```text
.temp/logs/<random>.txt
```

The path is relative to the `mcp/` module root. stdio stdout remains
protocol-only, so use this log directory when debugging Claude or Studio.

## Manual HTTP Check

Start the server:

```sh
make start ADDR=127.0.0.1:18090
```

Initialize:

```sh
curl -sS -X POST http://127.0.0.1:18090/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
```

List connections:

```sh
curl -sS http://127.0.0.1:18090/admin/connections
```

Open an SSE stream in another terminal:

```sh
curl -sS -N http://127.0.0.1:18090/mcp
```

The stream first emits:

```text
event: connection
data: {"id":"conn-1"}
```

Trigger inference for that connection:

```sh
curl -sS -X POST http://127.0.0.1:18090/admin/run_inference \
  -H 'Content-Type: application/json' \
  -d '{"connection_id":"conn-1"}'
```

The SSE stream receives a JSON-RPC `notifications/event` event by default.

## Delivery Mode

The default delivery mode is notification:

```sh
MCP_DELIVERY_MODE=notification make run
```

To deliver admin and scheduled prompts as sampling requests instead:

```sh
MCP_DELIVERY_MODE=sampling make run
```

Sampling mode emits JSON-RPC messages with method `sampling/createMessage`
instead of `notifications/event`.

Stop the server:

```sh
make stop
```
