# CLI Setup and Run

This document explains how to run the `cli_go/cli` package.

## Current status

- `cli_go/cli` is a library package.
- `cli_go/cli/cmd/main.go` is the executable entrypoint.
- It wires `operator.New()`, `config.DefaultOmniConfigResolver`, and `cli.Entrypoint(...).Install()`.

## Install

Using Go toolchain (recommended):

```bash
cd cli_go
go install ./cli/cmd
```

Or install directly from module path:

```bash
go install github.com/Shaik-Sirajuddin/memory/cli/cmd@latest
```

Using `curl` from release artifacts (when published):

```bash
VERSION=v0.1.0
OS=linux
ARCH=amd64

curl -fsSL \
  "https://github.com/Shaik-Sirajuddin/memory/releases/download/${VERSION}/omni_${OS}_${ARCH}.tar.gz" \
  | tar -xz

install -m 0755 omni /usr/local/bin/omni
```

## Run commands

From `cli_go/`:

```bash
go run ./cli/cmd --help
go run ./cli/cmd config get
go run ./cli/cmd config get --output table
go run ./cli/cmd config set --memory=true --autosync=true
go run ./cli/cmd agent discover
go run ./cli/cmd agent list --workspace /absolute/workspace/path
go run ./cli/cmd agent create --workspace /absolute/workspace/path --interactive=true
go run ./cli/cmd agent workspace-list
go run ./cli/cmd agent workspace-get --id <workspace-id> --output table
go run ./cli/cmd team-init
```

If installed globally:

```bash
omni --help
omni config get
omni config get --output yaml
omni agent discover
omni agent workspace-get --id <workspace-id> --output json
omni team-init
```

## Notes

- Config is stored by `DefaultOmniConfigResolver` at XDG config path:
  - `memory/omni/config.json`
- Agent commands delegate to `operator.Operator` methods.
