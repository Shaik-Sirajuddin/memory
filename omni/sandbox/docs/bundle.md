# OCI Bundle Format

## Required Layout

gVisor runtime creation expects `ProvisionerOptions.WorkDir` to point to an OCI bundle directory with this layout:

```text
<bundle-dir>/
  config.json
  rootfs/
    ...
```

## WorkDir Requirement

- Preferred: set `WorkDir` to the bundle directory itself.
- If `<WorkDir>/config.json` exists, gVisor uses that bundle.
- If `WorkDir` is not a valid OCI bundle, gVisor auto-creates a default global bundle at:
  - `$XDG_DATA_HOME/memory/sandboxes/gvisor/bundles/<sandbox-id>/config.json`
  - default `XDG_DATA_HOME` fallback: `~/.local/share`

## Example

```text
/home/user/sandboxes/demo/
  config.json
  rootfs/
```

Use `WorkDir=/home/user/sandboxes/demo` when you want explicit bundle control.

Reference templates:

- `sandbox/provider/gvisor/template/config.json`
- `sandbox/provider/gvisor/template/oci-bundle-layout.txt`

## Quick Setup

1. Create bundle dirs: `mkdir -p <bundle-dir>/rootfs`
2. Populate `rootfs` with container filesystem.
3. Generate spec from bundle dir: `runsc spec -- /bin/sh`
4. Use that bundle dir as `ProvisionerOptions.WorkDir`.
