# Configuration Reference

## Configuration Sources

| Scope | File | Usage |
|---|---|---|
| Workspace | `.gemini/settings.json` | Session-local sync target for model, approval mode, sandbox, and hook registration. |
| Global | `~/.gemini/settings.json` | Defaults source for model, approval mode, and sandbox fallback behavior. |

## Managed Settings

| Key | Purpose | Written By |
|---|---|---|
| `model` | Default model selection | `Create`, `UpdateDefaults` |
| `approvalMode` | Approval policy used by Gemini CLI | `Create`, `UpdateDefaults` |
| `sandbox` | Sandbox profile (`read-only` or full access) | `UpdateSessionSandbox`, `UpdateDefaults`, session sync |
| `hooks` | Hook registration definitions | `Register`, `DeleteHook`, hook sync |

## Environment and Runtime Expectations

| Requirement | Description |
|---|---|
| Gemini CLI in PATH | Connector initialization requires `gemini` binary discovery. |
| Writable workspace settings path | `.gemini/settings.json` is created or updated when session defaults/hooks change. |
| Writable user config path | `~/.gemini/settings.json` is updated when global defaults are changed. |

## User-Facing Behavior Notes

| Scenario | Behavior |
|---|---|
| Global settings unavailable | Connector continues with in-memory values and logs warning-level diagnostics. |
| Workspace settings missing | Connector creates workspace settings file during sync operations. |
| Unknown existing settings keys | Connector preserves unrelated keys while updating managed keys. |
