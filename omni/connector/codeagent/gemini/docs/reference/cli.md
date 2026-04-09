# CLI Reference

## Command Mapping

| Connector Operation | Gemini CLI Pattern | Notes |
|---|---|---|
| Version check | `gemini --version` | Used for binary reachability checks in initialization and session creation. |
| Identity availability check | `gemini --help` | Used as a lightweight access probe. |
| Exec | `gemini <prompt> ...flags` | Applies model, approval mode, system prompt, max turns, and ACP mode based on request. |
| Stream | `gemini <prompt> ...flags` with ACP mode | Line-based stream output is normalized to contract stream events. |
| Model discovery | `gemini models --json` then `gemini model list` | Falls back to static connector model list when unavailable. |

## Permission Mode Mapping

| CodeAgent Permission Mode | Gemini Approval Mode |
|---|---|
| `default` | `default` |
| `plan` | `plan` |
| `acceptEdits` | `auto_edit` |
| `auto` | `auto_edit` |
| `dontAsk` | `yolo` |
| `bypassPermissions` | `yolo` |

## Expected User Behavior

| User Action | Result |
|---|---|
| No explicit permission mode passed | Connector uses default `acceptEdits` and maps to Gemini `auto_edit`. |
| Explicit permission mode passed | Session uses mapped approval mode for that request flow. |
| Resume/list/delete operation requested | Connector uses Gemini session commands and returns explicit validation errors for invalid session IDs. |
