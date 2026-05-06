# Sandbox Doctor

## Purpose

`Doctor` provides a minimal runtime health check and Linux auto-install path for sandbox provisioning prerequisites.

## API

- `NewDoctor() *Doctor`
- `Doctor.Health() HealthStatus`
- `Doctor.Install() error`

## Health Detection

- Linux:
  - checks `runsc` for `gvisor`
  - when running rootless (non-zero euid), checks `newuidmap` and `newgidmap`
  - validates rootless helpers are setuid and root-owned
  - validates `/etc/subuid` and `/etc/subgid` contain an entry for the current user
- macOS: checks `sandbox-exec` for `seatbelt`

## Install Script

- Script path: `scripts/install-sandbox-runtime.sh`
- Linux:
  - downloads and installs `runsc` (gVisor runtime) if missing
  - installs uidmap helpers (`newuidmap`/`newgidmap`) needed for rootless gVisor
  - repairs uidmap helper ownership/permissions (`root:root`, `4755`)
  - ensures subordinate id mappings for the current user in `/etc/subuid` and `/etc/subgid`
- macOS: validates `sandbox-exec` presence (no installer path)

## Notes

- `Doctor.Install()` is a no-op when runtime is already installed.
- Current auto-install path is Linux-only because `runsc` is the Linux sandbox runtime dependency.
- `gvisor` runtime wrapping reports cgroup delegation errors with guidance; `omni` rootless runtime automatically adds `-ignore-cgroups`.
