# API Reference — CodeAgent Interface

The `CodeAgent` interface is the contract every provider connector must fulfill. It covers session lifecycle, prompt execution, streaming, sandbox control, hooks, and identity.

---

## Providers

| Constant | Value | CLI binary |
|---|---|---|
| Claude | `claude` | `claude` |
| Gemini | `gemini` | `gemini` |
| Codex | `codex` | `codex` |

---

## Permission Modes

Controls how aggressively the agent executes tools without prompting.

| Mode | Behavior | Claude | Gemini | Codex |
|---|---|---|---|---|
| `default` | Prompt for most tool uses | ✓ | ✓ | ✓ |
| `plan` | Read-only planning, no execution | ✓ | ✓ | ✓ |
| `acceptEdits` | Auto-accept file edits only | ✓ | — | ✓ |
| `auto` | Smart classifier decides | ✓ | `auto_edit` | — |
| `dontAsk` | Skip most prompts | ✓ | — | ✓ |
| `bypassPermissions` | Skip all permission checks | ✓ | `yolo` | ✓ |

---

## Output Formats

Applies to `Exec` and `Stream` methods.

| Format | Description |
|---|---|
| `text` | Plain text response |
| `json` | Structured JSON object |
| `stream-json` | Newline-delimited JSON events |

---

## Session Type

A `Session` represents a persisted agent conversation.

| Field | Type | Description |
|---|---|---|
| ID | string | Unique session identifier |
| Name | string | Human-readable display name |
| Provider | Provider | Which agent backend |
| Model | string | Model in use for this session |
| WorkDir | string | Working directory at session creation |

---

## Interface Methods

### Create

Initialises a new session. Providers without a CLI-level create command generate a client-side session ID.

| Parameter | Required | Description |
|---|---|---|
| ID | No | Custom session ID; auto-generated if empty |
| ParentID | No | Fork from an existing session |
| Model | No | Defaults to provider's default model |
| Name | No | Display name for the session |
| WorkDir | No | Sets the working directory; falls back to current dir |
| PermissionMode | No | See Permission Modes table |
| SystemPrompt | No | Replaces or augments the default system prompt |

| Result Field | Description |
|---|---|
| ID | Assigned session ID |
| Name | Assigned session name |

---

### Exec

Runs a prompt to completion and returns the full response. Blocking.

| Parameter | Required | Description |
|---|---|---|
| PromptId | No | Caller-assigned prompt tracking ID |
| Prompt | Yes | The prompt text to send |
| OutputFormat | No | text / json / stream-json |
| MaxTurns | No | Max agent turns; 0 = unlimited |

| Result Field | Description |
|---|---|
| PromptID | Echo of input PromptId |
| SessionID | Session that handled the prompt |
| Response | Full text response |
| StopReason | Why the agent stopped (e.g. "stop") |
| Usage | Token counts and estimated cost |

---

### Stream

Runs a prompt and returns an event channel immediately. Non-blocking. The caller must drain the channel.

| Parameter | Required | Description |
|---|---|---|
| PromptId | No | Caller-assigned tracking ID |
| Prompt | Yes | The prompt text |
| MaxTurns | No | Max agent turns; 0 = unlimited |

**Stream Events**

| Event Type | Meaning | Done flag |
|---|---|---|
| `text` | Incremental text content | false |
| `tool_use` | Agent is calling a tool | false |
| `tool_result` | Tool returned a result | false |
| `stop` | Stream ended | true |

> Check `Capabilities().Streaming` before calling this method.

---

### Resume

Resumes a previously saved session.

| Parameter | Description |
|---|---|
| ID | Session ID or name to resume |
| ForkSession | If true, creates a new session ID instead of reusing |

> Not supported by Codex — returns an error.

---

### List

Returns persisted sessions matching a filter.

| Parameter | Description |
|---|---|
| WorkDir | Filter by working directory |
| Provider | Filter by provider |

> Not supported by Codex — returns an empty list.

---

### Delete

Removes a persisted session by ID.

> Not supported by Codex — returns `Deleted: false`.

---

### GetSessionConfig / GetSessionSandbox / UpdateSessionSandbox

Read and write the active session's configuration and sandbox settings. `UpdateSessionSandbox` triggers a two-way sync to the provider's workspace config file.

---

### Capabilities

Returns a feature flag snapshot for the connected provider.

| Flag | Description |
|---|---|
| Hooks | Which hook events are supported |
| Streaming | Whether `Stream` is available |
| MCPSupport | Whether MCP servers can be configured |
| Worktrees | Whether git worktrees are supported |
| Subagents | Whether sub-agent spawning is supported |

---

### Info

Returns static metadata about the connected agent.

| Field | Description |
|---|---|
| Provider | Provider constant |
| Name | Binary/product name |
| Version | Installed CLI version |

---

### GetUserIdentity

Returns current authentication state.

| Field | Description |
|---|---|
| Authenticated | Whether the user is logged in |
| Email | Account email (where available) |
| DisplayName | Account display name (where available) |

---

## Usage / Token Accounting

| Field | Description |
|---|---|
| InputTokens | Tokens consumed in the prompt |
| OutputTokens | Tokens generated in the response |
| CostUSD | Estimated cost in US dollars |
