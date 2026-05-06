# Configuration Reference

## Configuration Sources

| Scope | File | Usage |
|---|---|---|
| Workspace | `.gemini/settings.json` | Session-local sync target for model, approval mode, sandbox, and hook registration. |
| Global | `~/.gemini/settings.json` | Defaults source for model, approval mode, and sandbox fallback behavior. |

## Managed Settings

| Key | Purpose | Written By |
|---|---|---|
| `model.name` | Default model selection | `Create`, `UpdateDefaults` |
| `general.defaultApprovalMode` | Approval policy used by Gemini CLI | `Create`, `UpdateDefaults` |
| `tools.sandbox` | Sandbox profile (`read-only` or full access) | `UpdateSessionSandbox`, `UpdateDefaults`, session sync |
| `hooks.<event>` | Hook registration definitions keyed by Gemini hook event | `Register`, `DeleteHook`, hook sync |

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
| Settings schema | Connector reads and writes Gemini settings through the generated `SettingsSchemaJson` schema. Unsupported unknown fields may be omitted when a settings file is rewritten. |
