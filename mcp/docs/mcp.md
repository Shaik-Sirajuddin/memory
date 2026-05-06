# Minimal MCP Server

This module contains a small Model Context Protocol server implementation for
local testing of streamable HTTP connections and server-initiated sampling
requests.

## Transports

### Streamable HTTP

- `POST /mcp` accepts JSON-RPC requests and responses.
- `GET /mcp` opens a Server-Sent Events stream.
- Each open stream is registered as a connection and receives an
  `Mcp-Session-Id` response header.
- The server sends queued JSON-RPC messages over the SSE stream as
  `event: message` events.

### stdio

The `stdio` transport reads newline-delimited JSON-RPC messages from stdin and
writes newline-delimited JSON-RPC messages to stdout. Logs are written to a file
so stdout remains protocol-only.

The implementation is intentionally minimal. It supports initialization, ping,
initialized notifications, sampling responses, connection listing, and
server-triggered sampling requests.

## JSON-RPC Methods

### `initialize`

Request:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize"
}
```

Response includes:

- `protocolVersion: "2025-03-26"`
- `serverInfo.name: "memory-tree-minimal-mcp"`
- minimal `tools` and `resources` capabilities

### `notifications/initialized`

Accepted as a notification after initialization. The server returns
`202 Accepted`.

### `ping`

Returns an empty JSON-RPC result.

### Discovery

The server currently advertises no tools, resources, resource templates, or
prompts. These discovery methods return empty successful results:

- `tools/list`
- `resources/list`
- `resources/templates/list`
- `prompts/list`

### Sampling Responses

When the client posts a JSON-RPC response without a `method`, the server logs it
as the response to a previous `sampling/createMessage` request.

## Admin Endpoints

### `GET /admin/connections`

Lists currently open SSE connections.

```json
{
  "connections": [
    {
      "id": "conn-1",
      "remote_addr": "127.0.0.1:35428",
      "user_agent": "curl/8.5.0",
      "opened_at": "2026-05-02T08:54:22.659782355Z"
    }
  ]
}
```

### `POST /admin/run_inference`

Queues an inference delivery to all open connections, or to one connection when
`connection_id` is provided.

Request:

```json
{
  "connection_id": "conn-1"
}
```

Response:

```json
{
  "sent": ["conn-1"]
}
```

## Scheduled Prompt

The server sends a cat-command example prompt on a schedule. Streamable HTTP
sends to all open SSE connections. stdio writes to stdout for the connected
client.

```text
Run this example command and return the output: cat docs/setup.md
```

- HTTP default: `1m`
- stdio default: `10s`

Override with the `-interval` flag or the Makefile `INTERVAL` /
`STDIO_INTERVAL` variables.

## Delivery Mode

`MCP_DELIVERY_MODE` controls how inference delivery is encoded.

- `notification`, `event`, `notifications/event`, or unset: JSON-RPC notification
  method `notifications/event`
- `sampling`: JSON-RPC request method `sampling/createMessage`

Example:

```sh
MCP_DELIVERY_MODE=sampling make run
```

## Logging

Logging uses the shared `log/slog` logger in `mcp/log/logger.go`.

- default level: `INFO`
- debug level: set `DEV=1`
- default log path: `.temp/logs/<random>.txt` under the `mcp/` module root
