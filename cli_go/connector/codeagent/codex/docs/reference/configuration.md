# Configuration — Codex Connector

## Environment variables

| Variable | Required | Purpose |
|---|---|---|
| `OPENAI_API_KEY` | Yes (for API use) | Authenticates requests to OpenAI; also required for `FetchModels` |

Authentication state is determined by the presence of `OPENAI_API_KEY`. There is no interactive login flow accessible from the connector — use `codex --login` in a terminal if needed.

---

## Config files

| Scope | Path | Written by connector |
|---|---|---|
| User | `~/.codex/config.yaml` | No — managed by user or CLI |
| Project | `.codex/config.yaml` | Yes — on `UpdateSessionSandbox` |

### What the connector writes

When `UpdateSessionSandbox` is called, the connector writes or updates the `sandbox` key in `.codex/config.yaml` inside the active working directory. Existing unrelated keys in the file are preserved.

| Scenario | Written value |
|---|---|
| Sandbox cleared (nil) | `sandbox` key removed |
| Read-only sandbox | `sandbox: read-only` |
| Full-access sandbox | `sandbox: danger-full-access` |

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
