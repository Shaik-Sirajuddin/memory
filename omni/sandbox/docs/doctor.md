# Sandbox Doctor

## Purpose

`Doctor` provides a minimal runtime health check and Linux auto-install path for sandbox provisioning prerequisites.

## API

- `NewDoctor() *Doctor`
- `Doctor.Health() HealthStatus`
- `Doctor.Install() error`

## Health Detection

- Linux: checks `runsc` for `gvisor`
- macOS: checks `sandbox-exec` for `seatbelt`

## Install Script

- Script path: `scripts/install-sandbox-runtime.sh`
- Linux: downloads and installs `runsc` (gVisor runtime) if missing
- macOS: validates `sandbox-exec` presence (no installer path)

## Notes

- `Doctor.Install()` is a no-op when runtime is already installed.
- Current auto-install path is Linux-only because `runsc` is the Linux sandbox runtime dependency.
