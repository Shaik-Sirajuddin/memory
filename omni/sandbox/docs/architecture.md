# Sandbox Architecture

## Intended Package Shape

The sandbox package is intended to expose a stable top-level API while organizing concrete runtimes by provider.

Target structure:

```text
omni/sandbox/
  sandbox.go
  types.go
  store.go
  provider/
    gvisor/
      default.go
    bubblewrap/
      default.go
    seatbelt/
      default.go
```

## Architectural Boundaries

### Top-Level `sandbox` Package

Responsible for:

- public types
- public interfaces
- provider selection
- host capability mapping
- stable import path for the rest of the system

### Provider Implementations

Responsible for:

- translating sandbox config into runtime-specific behavior
- runtime command construction
- provider-local lifecycle handling
- provider-specific limitations and defaults

## Configuration Model

Filesystem isolation is driven primarily by:

- `WorkSpacePolicy`
- `AgentPolicy`
- `MountConfig`

Important policy dimensions:

- allowed directories
- blocked directories
- write-permitted versus read-only operation

## Lifecycle Expectations

Every provider should map to the same external lifecycle shape:

1. `Create` registers or provisions a sandbox
2. `Command` runs an attached interactive command path
3. `Execute` runs a non-interactive command path
4. `Sync` applies updated config
5. `List` returns known sandboxes
6. `GetSandbox` resolves a sandbox by pid or logical id

## Expansion Areas

- persistent provider-backed store integration
- gVisor OCI bundle support
- richer network policy controls
- tool-aware sandbox expansion driven by operator or codeagent needs
