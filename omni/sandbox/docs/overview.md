# Sandbox Overview

## Purpose

The `sandbox` package provides a common abstraction for isolated command execution across host platforms.

Its role is to let higher-level components configure and operate execution sandboxes without coupling directly to a specific host runtime such as:

- `gVisor` on Linux
- `bubblewrap` on Linux
- `seatbelt` on macOS

The package is intended to support:

- sandbox creation and lookup
- command execution inside a provisioned sandbox
- sandbox configuration sync
- provider selection by host platform

## Core Model

The sandbox model is built around three layers:

- `Config`
  - filesystem and workspace policy for a sandbox
- `State`
  - runtime state such as process identity and active status
- `Data`
  - metadata such as sandbox id, application name, and creation time

These are composed into `Sandbox`.

## Main API Intent

The central runtime abstraction is `SandboxProvisioner`.

It is intended to provide a lifecycle-oriented interface:

- `Create`
- `Command`
- `Execute`
- `Sync`
- `List`
- `GetSandbox`

This keeps provider-specific logic behind a single interface while allowing each backend to adapt to its host runtime model.

## Platform Direction

- Linux
  - primary direction is `gVisor` and `bubblewrap`
- macOS
  - primary direction is `seatbelt`
- Windows
  - expected operational path is Linux semantics through WSL2 rather than a separate native provider in the first phase

## Current Product Direction

The sandbox package is moving toward a provider-oriented layout so each runtime backend can live under its own implementation folder while the top-level package remains the stable API exposed to the rest of the codebase.
