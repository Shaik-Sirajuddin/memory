# OmniAgent Architecture

## What is OmniAgent?

OmniAgent is an agent runtime that manages one or more **code-agent sessions** within a single working directory. It wraps external code agents (Claude, Codex, Gemini) behind a uniform interface and keeps a persistent record of sessions, settings, and memory.

The key constraint: **an OmniAgent instance is bound to one working directory at a time**. Forking into a parallel instance is supported, but each fork owns its own state.

---

## Layers

```
┌─────────────────────────────────────┐
│           OmniAgent (interface)     │  ← runtime lifecycle, hook entrypoints
├─────────────────────────────────────┤
│       OmniAgentStore (interface)    │  ← agent-level persistence
│       CodeSessionStore (interface)  │  ← session-level persistence
├─────────────────────────────────────┤
│     SQLite via modernc.org/sqlite   │  ← single file at XDG data path
└─────────────────────────────────────┘
```

### OmniAgent (runtime)
The `OmniAgent` interface is the top-level handle. It holds live state (`Data`) and exposes operations: creating sessions, syncing memory, and updating settings. Settings changes are deferred — they apply after the current command finishes, so an in-flight prompt sees a consistent configuration.

### Store layer
The store layer is intentionally split:

- **`OmniAgentStore`** owns agent-level records: identity (`AgentInfo`), settings, and the active session pointer. It delegates all session reads and writes to `CodeSessionStore`.
- **`CodeSessionStore`** is agent-independent — every method takes an explicit `agentID`. This makes it composable: `OmniAgentStore` uses it internally, but it can also be called directly when only session data is needed.

Both stores are **singletons** backed by `sync.Once`. The database connection is also a singleton, initialised once and shared.

### Database
A single SQLite file holds three tables:

| Table | Owns |
|---|---|
| `agents` | `AgentInfo` (id, name, workspace, memory dir) |
| `code_sessions` | `CodeSession` per agent (model stored as flat columns) |
| `agent_settings` | `Settings` per agent (sandbox as JSON, model as flat columns) |

Complex nested values (`sandbox.Config`) are JSON-serialised in-place. Primitive and flat values (model provider/name) use direct SQLite columns for easier querying and to avoid unmarshalling overhead.

---

## Data flow: creating an agent

```
caller
  │
  ├─ store.GetOmniAgentStore()   ← singleton, initialises DB + CodeSessionStore
  │
  ├─ agentStore.Create(&Data{…}) ← inserts into `agents` + `agent_settings`
  │
  └─ agentStore.CreateSession(id, &CodeSession{…})
         │
         └─ delegated to CodeSessionStore.CreateSession(id, session)
                │
                └─ inserts into `code_sessions`
```

---

## Sessions vs. active session

An agent may have many sessions over its lifetime, but only one is **active** at a time (`IsActive = true`). `GetActiveSession` queries `code_sessions` where `is_active = 1`. `SyncMemory` on the runtime flushes the live session state back to the store via `UpdateActiveSession`.

---

## Hook entrypoints

`OmniAgentEntrypoint` provides six hook points around each prompt/tool cycle:

```
PreSessionStart → PrePrompt → [prompt] → PostPrompt
                           → PreToolUse → [tool] → PostToolUse
PostSessionStart
```

These are intended for middleware that needs to observe or modify agent behaviour — e.g. injecting context before a prompt, or recording tool usage after the fact.
