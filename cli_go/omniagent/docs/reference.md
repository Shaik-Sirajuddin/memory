# OmniAgent Reference

## Core Types

### `AgentInfo`
Identity and location of an agent instance.

| Field | Type | Description |
|---|---|---|
| `ID` | `string` | Unique agent identifier |
| `Name` | `string` | Human-readable name |
| `WorkspaceDir` | `sandbox.WorkspaceDir` | Bound workspace directory key |
| `MemoryDir` | `string` | Absolute path for agent-specific files |

---

### `CodeSession`
A persisted code-agent session belonging to an agent.

| Field | Type | Description |
|---|---|---|
| `Id` | `string` | Unique session identifier |
| `Model` | `*codeagent.Model` | Provider and model used for this session |
| `Idx` | `int` | Ordered index within the agent's sessions |
| `IsActive` | `bool` | Whether this is the current active session |
| `Prompts` | `int` | Total prompts executed in this session |
| `LastSyncPrompt` | `int` | Prompt index at last memory sync |

---

### `Settings`
Runtime configuration for an agent (defined in `omniagent/config`).

| Field | JSON Key | Description |
|---|---|---|
| `Model` | `model` | AI model identifier shared across all supported agents |
| `Timeout` | `timeout` | Per-hook / per-invocation timeout in seconds |
| `MaxTurns` | `maxTurns` | Maximum agentic turns allowed per prompt execution |
| `Sandbox` | `sandbox` | Execution sandbox / permission mode (e.g. `read-only`, `danger-full-access`) |
| `Env` | `env` | Environment variables injected into the agent process |
| `Hooks` | `hooks` | Lifecycle hook commands mapped by event name |
| `Theme` | `theme` | UI theme name (e.g. `dark`, `light`, custom theme id) |
| `Cwd` | `cwd` | Working directory for agent and tool processes |
| `LogPrompts` | `logPrompts` | Whether user prompts are logged in session output |

### `HookEntry`
A single hook command registered for a lifecycle event.

| Field | JSON Key | Description |
|---|---|---|
| `Command` | `command` | Shell command to run when the hook fires |
| `Args` | `args` | Additional arguments passed to the command |
| `Timeout` | `timeout` | Per-hook timeout override (seconds) |
| `Url` | `url` | HTTP endpoint for webhook-style hooks |

### `Config`
Agent-level configuration embedding hook capabilities.

| Field | Type | Description |
|---|---|---|
| `Capabilities` | `hooks.Capabilities` | Declares which lifecycle hooks the agent supports |

---

### `Data`
Full in-memory state of a running agent. Array fields (`Sessions`) are omitted when fetched from the store.

| Field | Type | Description |
|---|---|---|
| `Info` | `*AgentInfo` | Agent identity |
| `ActiveWorkSpace` | `*sandbox.Workspace` | Currently active workspace |
| `ActiveSession` | `*CodeSession` | Currently active session |
| `Settings` | `*Settings` | Agent settings |
| `Sessions` | `[]*CodeSession` | All sessions (runtime only, not fetched from store) |

---

## Interfaces

### `OmniAgent`
Controls the lifecycle of a running agent instance.

```go
type OmniAgent interface {
    Data() *Data
    New()
    UpdateSettings(UpdateSettingsParams) error
    SyncMemory()
    NewCodeSession()
}
```

| Method | Description |
|---|---|
| `Data()` | Returns current in-memory state |
| `New()` | Initialises a new agent instance |
| `UpdateSettings(params)` | Applies settings after the current command completes |
| `SyncMemory()` | Flushes session memory to the persistent store |
| `NewCodeSession()` | Creates a new code session, optionally with a different model |

---

### `OmniAgentEntrypoint`
Hook points called around each prompt/tool interaction.

```go
type OmniAgentEntrypoint interface {
    PreToolUse()
    PostToolUse()
    PrePrompt()
    PostPrompt()
    PreSessionStart()
    PostSessionStart()
}
```

---

## Store Interfaces

### `OmniAgentStore`
Persistent storage for agent records. Obtain via `store.GetOmniAgentStore()`.

```go
store, err := store.GetOmniAgentStore()
```

| Method | Description |
|---|---|
| `Create(agent *Data) error` | Insert a new agent with info and default settings |
| `Save(agent *Data) error` | Upsert scalar fields (info + settings); array fields ignored |
| `GetAgent(ID string) (*Data, error)` | Fetch agent info and settings; `Sessions` is omitted |
| `GetActiveSession(ID string) (*CodeSession, error)` | Return the active session for an agent |
| `UpdateActiveSession(ID string, session *CodeSession) error` | Persist changes to the active session |
| `CreateSession(ID string, session *CodeSession) error` | Persist a new session for an agent |
| `GetSettings(ID string) (*Settings, error)` | Fetch settings for an agent |
| `UpdateSettings(ID string, settings *Settings) error` | Upsert settings for an agent |
| `ListAgents(params ListAgentParams)` | Query agents filtered by workspace |
| `DeleteAgent()` | Remove agent from the index (data is retained) |

#### `ListAgentParams`

| Field | Type | Description |
|---|---|---|
| `Workspace` | `sandbox.WorkspaceDir` | Filter agents by workspace key |

---

### `CodeSessionStore`
Persistent storage for code sessions, scoped per call by `agentID`. Obtain via `store.GetCodeSessionStore()`.

```go
sessions, err := store.GetCodeSessionStore()
```

| Method | Description |
|---|---|
| `GetSession(agentID string) (*CodeSession, error)` | Return the active session for the agent |
| `CreateSession(agentID string, session *CodeSession) error` | Insert a new session |
| `UpdateSession(agentID string, session *CodeSession) error` | Update an existing session |
| `ListSessions(agentID string, filter *CodeSession) ([]*CodeSession, error)` | List sessions; filter on non-zero fields of `filter` |

---

## Factory Functions

Both stores are singletons — safe to call from multiple goroutines.

```go
// Returns the singleton OmniAgentStore (initialises DB on first call).
agentStore, err := store.GetOmniAgentStore()

// Returns the singleton CodeSessionStore.
sessionStore, err := store.GetCodeSessionStore()
```

The underlying SQLite database is stored at the XDG data path:
```
$XDG_DATA_HOME/memory/omniagent.db   # typically ~/.local/share/memory/omniagent.db
```
