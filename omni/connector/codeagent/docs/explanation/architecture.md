# Architecture

## Package layout

```
connector/
├── codeagent/              ← shared interface + types
│   ├── hooks/              ← hook event system
│   ├── claude/             ← hook schemas + CLI spec
│   ├── gemini/             ← hook schemas + CLI spec
│   └── codex/              ← hook schemas + CLI spec + Go implementation
└── sandbox/                ← sandbox policy types
```

---

## Layers

| Layer | Location | Responsibility |
|---|---|---|
| Interface | `codeagent.go` | Defines `CodeAgent` contract and all shared types |
| Hook system | `hooks/` | Hook IDs, common input/output schema, parser interfaces |
| Hook schemas | `<provider>/hooks/` | JSON Schema files per event per provider |
| CLI specs | `<provider>/cli/openapi.yaml` | Human-readable command / flag / config reference |
| Implementation | `codex/codex.go` | Concrete `CodeAgent` backed by the Codex CLI binary |
| Sandbox | `sandbox/sandbox.go` | Workspace and file policy types |

---

## Control flow — Exec

1. Caller invokes `Exec` with a prompt and options
2. Connector reads working directory and model from internal state (mutex-guarded)
3. Connector maps `OutputFormat` and `MaxTurns` to CLI flags
4. Connector maps `sandbox.Config` to provider sandbox flag (`read-only` / `danger-full-access` for Codex)
5. CLI process is launched with working directory set
6. Stdout is captured; stderr is captured for error messages
7. Exit code is inspected — non-zero produces a traced error including exit code and stderr content
8. Result is returned with `StopReason` and `SessionID`

---

## Control flow — Stream

1. Caller invokes `Stream` — returns immediately with a channel
2. Connector starts the CLI process with JSON output mode
3. A background goroutine scans stdout line by line
4. Each JSON line is decoded into a `StreamEvent` (type + content)
5. Plain-text lines are emitted as `type=text` events
6. On process exit, a final `type=stop, Done=true` event is sent and the channel is closed
7. Errors during the process are included in the final stop event's `Content` field

---

## Control flow — Sandbox two-way sync

| Direction | Trigger | What happens |
|---|---|---|
| Write (outbound) | `UpdateSessionSandbox` called | In-memory state updated → `.codex/config.yaml` written in workdir |
| Read (inbound) | `GetSessionSandbox` called | Returns current in-memory state |

The sandbox flag written to config is derived from the `sandbox.Config` struct:

| Sandbox state | Flag written |
|---|---|
| `nil` (no sandbox) | Key removed from config |
| `ExtendedPolicy` set | `danger-full-access` |
| `ExtendedPolicy` nil | `read-only` |

---

## Hook system

### Common input fields (all providers)

| Field | Description |
|---|---|
| `session_id` | Unique session identifier |
| `transcript_path` | Path to the session transcript file |
| `cwd` | Current working directory at hook fire time |
| `hook_event_name` | The event that fired (e.g. `PreToolUse`) |

### Common output fields (all providers)

| Field | Default | Description |
|---|---|---|
| `continue` | true | Set false to halt the agent loop |
| `stopReason` | null | Message shown when `continue` is false |
| `suppressOutput` | false | Hide hook metadata from terminal output |
| `systemMessage` | null | Inject a message into the conversation |

### Hook event support by provider

| Event | Codex | Claude | Gemini |
|---|---|---|---|
| PreToolUse / PostToolUse | ✓ | ✓ | BeforeTool / AfterTool |
| PostToolUseFailure | ✓ | ✓ | — |
| SessionStart | ✓ | ✓ | ✓ |
| UserPromptSubmit | ✓ | ✓ | BeforeAgent |
| Stop | ✓ | ✓ | AfterAgent |
| Notification | — | ✓ | ✓ |
| BeforeModel / AfterModel | — | — | ✓ |
| PreCompact / PostCompact | — | ✓ | PreCompress |
| SubagentStart / Stop | — | ✓ | — |

---

## Error tracing convention

All errors carry a structured prefix identifying the connector and operation, wrapping the original cause. This allows callers to inspect the full error chain.

Format: `<connector>: <operation>: <original error>`

CLI exit errors additionally include the exit code and the stderr text from the failed process.

---

## Concurrency

The `codexAgent` struct uses a `sync.RWMutex` to guard mutable state (`workDir`, `model`, `sandbox`). Read operations take a read lock; write operations (e.g. `Create`, `UpdateSessionSandbox`) take a write lock. All public methods are safe to call from multiple goroutines.
