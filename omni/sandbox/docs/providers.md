# Sandbox Providers

## gVisor

### Target

- Linux
- Linux environments reached through WSL2

### Runtime

- `runsc`

### Why It Exists

`gVisor` provides stronger isolation than a plain process wrapper by placing execution behind a user-space kernel boundary.

### Expected Capabilities

- create sandbox/container instances from provisioner-managed metadata
- execute commands via `runsc exec`
- inspect or list provisioned sandboxes
- support future config sync using runtime metadata or OCI bundle updates

### Notes

- best fit for long-lived or higher-risk isolated sessions
- natural home for Linux-first production sandboxing

## bubblewrap

### Target

- Linux

### Runtime

- `bwrap`

### Why It Exists

`bubblewrap` is lightweight and practical for process isolation on Linux hosts where a full OCI runtime or microVM layer is unnecessary.

### Expected Capabilities

- mount shaping based on sandbox filesystem policy
- process-level isolation for direct command execution
- fast startup for shorter-lived commands

### Notes

- weaker boundary than gVisor
- useful as a default local Linux option

## seatbelt

### Target

- macOS

### Runtime

- `sandbox-exec` and Seatbelt profile rules

### Why It Exists

`seatbelt` provides the macOS-native path for filesystem-constrained execution where Linux-specific runtimes are not available.

### Expected Capabilities

- inline profile generation from sandbox policy
- command execution under a generated profile
- limited lifecycle tracking compared to Linux container-oriented runtimes

### Notes

- feature parity with Linux providers is not expected
- best treated as a host-native compatibility provider
