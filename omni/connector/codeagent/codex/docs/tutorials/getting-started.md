# Getting Started — Codex Connector

## Prerequisites

| Requirement | How to satisfy |
|---|---|
| Codex CLI installed | `npm install -g @openai/codex` |
| `codex` on PATH | Verify with `codex --version` in terminal |
| OpenAI API key | Set `OPENAI_API_KEY` environment variable |

---

## Creating an agent

Instantiate the connector with a working directory and an optional model name.

- If no working directory is given, the process current directory is used.
- If no model is given, `o4-mini` is used.
- The constructor verifies the `codex` binary exists and returns an error if not found.

---

## Running a prompt

Use `Exec` for a blocking, single-response call. The prompt is sent to `codex exec` under the hood.

**Expected behaviour:**
- The agent processes the prompt and returns when done.
- `StopReason` will be `"stop"` on normal completion.
- If the CLI exits with a non-zero code, the error includes the exit code and the CLI's stderr output.

---

## Streaming a response

Use `Stream` to receive incremental output via a channel.

**Expected behaviour:**
- The method returns immediately.
- Events arrive on the channel as the CLI produces output.
- The final event has `Done = true` and `Type = "stop"`.
- The caller must fully drain the channel before discarding it.

**Event types:**

| Type | Meaning |
|---|---|
| `text` | Partial response text |
| `tool_use` | Agent is invoking a tool |
| `tool_result` | Tool returned a result |
| `stop` | Stream complete — `Done` is true |

---

## Changing the sandbox mode

Update the sandbox at any time via `UpdateSessionSandbox`. The change is:
1. Applied immediately to the in-memory agent state.
2. Written to `.codex/config.yaml` in the working directory so future interactive sessions inherit it.

**Sandbox modes:**

| Sandbox setting | Written to config as |
|---|---|
| No sandbox (nil) | Sandbox key removed |
| Standard (no extended policy) | `read-only` |
| Extended policy set | `danger-full-access` |

---

## Fetching available models

Call `FetchModels` to get a live list from the OpenAI models API. This requires `OPENAI_API_KEY` to be set. If the API call fails for any reason, the static built-in list is returned instead — no error is surfaced to the caller.

---

## Known limitations

| Feature | Status |
|---|---|
| Session resume | Not supported — Codex has no CLI-level resume command |
| List sessions | Not supported — Codex has no session list API |
| Delete session | Not supported |
| MCP server configuration | Not supported by Codex |
| Worktrees | Not supported by Codex |
| Sub-agents | Not supported by Codex |
| `auto` permission mode | Not available — Codex uses `default`, `plan`, `acceptEdits`, `dontAsk`, `bypassPermissions` |
