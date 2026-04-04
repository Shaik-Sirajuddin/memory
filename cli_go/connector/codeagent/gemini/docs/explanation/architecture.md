# Architecture

## Component Layout

| File | Responsibility |
|---|---|
| `gemini.go` | Connector identity, defaults, capabilities, model discovery, and hook manager orchestration. |
| `commands.go` | Session lifecycle operations and prompt execution/stream command handling. |
| `parser.go` | Stream-line normalization and hook input/output payload parsing helpers. |
| `hooks.go` | Hook settings schema mapping, persistence, and conversion between stored and interface hook models. |
| `sandbox.go` | Sandbox state mapping and workspace config synchronization. |

## Runtime Control Flow

| Phase | Flow |
|---|---|
| Initialization | Resolve Gemini binary, default work directory, and default model; publish connector info metadata. |
| Session creation | Apply request overrides, persist session identity, validate CLI availability, sync workspace settings. |
| Execution | Build command args from model, permission mode, prompt settings, and sandbox; run Gemini command. |
| Streaming | Run Gemini with stream-compatible mode and normalize each line to contract stream events. |
| Settings synchronization | Keep in-memory state and workspace/global settings aligned for model, permission mode, sandbox, and hooks. |

## Concurrency Model

| Area | Strategy |
|---|---|
| Session state | Read/write locks guard workdir, model, permission mode, sandbox, session ID, and hook list. |
| Reads during execution | Execution captures a stable snapshot of required state under read lock before command launch. |
| Hook list changes | Register/delete operations mutate under write lock and then sync persisted settings. |
