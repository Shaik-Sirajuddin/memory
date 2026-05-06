# API Reference — Codex Connector

## Available models

The connector ships a static list of known Codex-compatible models.

| Model ID | Notes |
|---|---|
| `o4-mini` | **Default** — fast and cost-efficient |
| `o3` | Higher reasoning capability |
| `gpt-4.1` | Latest GPT-4 generation |
| `gpt-4.1-mini` | Smaller GPT-4.1 variant |
| `gpt-4o` | Multimodal GPT-4o |

The static list is used when no API key is present or when `FetchModels` fails.

---

## FetchModels

Contacts the OpenAI models API and returns the live list of available model IDs.

| Behaviour | Condition |
|---|---|
| Returns live list | `OPENAI_API_KEY` is set and API responds successfully |
| Returns static list | `OPENAI_API_KEY` is missing |
| Returns static list | API request or parse fails (error is logged, not returned) |

---

## Sandbox mapping

Codex supports two sandbox levels passed via `--sandbox` flag.

| `sandbox.Config` state | Codex flag | Effect |
|---|---|---|
| `nil` | *(not passed)* | No sandbox restrictions |
| Sandbox set, no ExtendedPolicy | `read-only` | Agent can read but not write to filesystem |
| Sandbox set, ExtendedPolicy present | `danger-full-access` | Full filesystem and network access |

---

## Logging

The connector uses a package-level structured logger (`slog`) writing to stderr.

| Log level | When used |
|---|---|
| Debug | Method entry, workdir/model values, command args, response lengths |
| Info | Session created, sandbox updated, models fetched |
| Warn | Unsupported operations (Resume, List, Delete), missing API key |
| Error | CLI process failures, config write failures, API errors |

Each log entry carries a `connector=codex` attribute for easy filtering.

---

## Method behaviour summary

| Method | Behaviour |
|---|---|
| `New` | Verifies binary on PATH, reads version, returns agent or error |
| `Create` | Updates workdir/model, returns generated session ID |
| `Exec` | Runs `codex exec <prompt>` — blocking, returns full response |
| `Stream` | Runs `codex exec <prompt> --json` — returns event channel |
| `Resume` | Always returns an error — not supported by Codex CLI |
| `List` | Always returns empty list — no session list API in Codex |
| `Delete` | Always returns `Deleted: false` — not supported |
| `Stop` | No-op — Codex non-interactive sessions self-terminate |
| `GetSessionConfig` | Returns current in-memory model, workdir, permission mode |
| `GetSessionSandbox` | Returns current in-memory sandbox state |
| `UpdateSessionSandbox` | Updates in-memory state and writes `.codex/config.yaml` |
| `Capabilities` | Returns static capability flags for Codex |
| `Config` | Returns current provider, model, permission mode, sandbox |
| `Info` | Returns provider=codex, name, installed version |
| `GetUserIdentity` | Returns authenticated=true if `OPENAI_API_KEY` is set |
| `FetchModels` | Returns live or static model list (see above) |
