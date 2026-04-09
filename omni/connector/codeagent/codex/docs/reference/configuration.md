# Configuration — Codex Connector

## Authentication

Authentication is verified by running `codex auth status`. Exit code 0 means logged in. The connector never reads API keys or credential files directly.

| How to authenticate | Command |
|---|---|
| Interactive login | `codex auth login` in a terminal |
| Check status | `codex auth status` |

---

## Config files

| Scope | Path | Managed by |
|---|---|---|
| User (global defaults) | `~/.codex/config.yaml` | `Defaults`, `UpdateDefaults`, `SaveDefaultSettings` |
| Workspace | `<workDir>/.codex/config.yaml` | `Create` (model sync), `UpdateSessionSandbox` |

### Keys written by the connector

All writes merge with existing file content — unrelated keys are always preserved.

| Key | Written on | Values |
|---|---|---|
| `model` | `Create`, `UpdateDefaults`, `SaveDefaultSettings` | model ID string (e.g. `o4-mini`) |
| `sandbox_mode` | `UpdateDefaults`, `SaveDefaultSettings` | `read-only`, `danger-full-access`, or removed when nil |

### Reading defaults (first-resolved order)

When `Defaults` is called, settings are sourced from `~/.codex/config.yaml`. If the file is missing or a key is absent, the in-memory value set at construction is used as the fallback.

---

## Settings resolver

The `SettingsResolver` provides structured access to config file state beyond the basic `Defaults` / `UpdateDefaults` API.

| Method | What it does |
|---|---|
| Get User Settings | Reads `~/.codex/config.yaml` and returns model and sandbox as typed values |
| Get Workspace Settings | Reads `<workspaceDir>/.codex/config.yaml` for the given workspace |
| Save Default Settings | Merges the given settings into `~/.codex/config.yaml`, preserving other keys |
| Watch Default Settings | Polls `~/.codex/config.yaml` every second; calls your callback when the file changes |

The watcher runs in a background goroutine. Starting a second watch automatically stops the previous one.

---

## Sandbox policy mapping

| Sandbox state | Config file value | Behaviour |
|---|---|---|
| None (nil) | key removed | No sandbox restriction |
| Read-only | `read-only` | Permissive read access policy |
| Full access | `danger-full-access` | All-permissive agent policy |

---

## Hooks configuration

Hooks for Codex are declared in the `hooks` section of the config file. The schema is defined in `codex/hooks/hooks-config.schema.json`.

### Supported hook events

| Event | Fires when |
|---|---|
| `PreToolUse` | Before a tool is called |
| `PostToolUse` | After a tool completes successfully |
| `SessionStart` | A session begins or resumes |
| `UserPromptSubmit` | User submits a prompt |
| `Stop` | The agent finishes responding |

### Hook exit codes

| Exit code | Meaning |
|---|---|
| `0` | Success — stdout parsed as JSON response |
| `2` | Block — stderr content used as rejection reason |
| Other | Warning — CLI continues, stderr shown in verbose mode |

---

## Logging configuration

The connector logs to stderr at `DEBUG` level by default. To reduce noise in production, set the `slog` level in your application to `INFO` or higher. The `connector=codex` attribute on every log line allows easy filtering with structured log tools.
