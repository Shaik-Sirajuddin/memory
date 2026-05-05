# API Reference

## Session and Execution Surface

| Interface Area | Methods | Gemini Connector Behavior |
|---|---|---|
| Session lifecycle | Create, Resume, List, Delete, Stop | Create resolves work directory, model, permission mode, syncs local settings, checks Gemini CLI reachability, seeds a prompt session, then returns the session ID discovered from `gemini --list-sessions`. Resume attaches to an interactive Gemini session. List and Delete use Gemini session commands. Stop terminates the active resumed process when present. |
| Prompt execution | Exec, Stream | Exec runs Gemini with resolved arguments and returns final response text. Stream runs Gemini and emits normalized stream events (`text`, `tool_use`, `tool_result`, `stop`). |
| Session config | GetSessionConfig | Returns current provider, model, permission mode, working directory, and system prompt. |
| Sandbox | GetSessionSandbox, UpdateSessionSandbox | Keeps session sandbox in memory and synchronizes workspace Gemini settings after updates. |
| Defaults and capabilities | Defaults, UpdateDefaults, Capabilities | Defaults resolve global Gemini settings with in-memory fallback. UpdateDefaults persists model, permission behavior, and sandbox policy to global settings. Capabilities report hooks, streaming, MCP, worktree, and subagent support flags. |
| Hook integration | SupportedHooks, Register, GetRegisteredHooks, DeleteHook and HookIOParser methods | Supports hook registration, listing, deletion, and payload parsing for tool, prompt, and session hook events. |
| Identity and info | Info, GetUserIdentity | Info returns provider metadata. Identity checks Gemini CLI availability and reports authenticated state accordingly. |

## Output Behavior

| Output Mode | Behavior |
|---|---|
| `text` | Returns plain text response from Gemini command output. |
| `json` | Requests ACP-compatible mode and returns JSON-like output text for caller parsing. |
| `stream-json` | Enables ACP mode and maps line-level stream events into contract stream events. |
