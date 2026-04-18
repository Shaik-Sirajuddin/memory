# CLI Setup and Run

This document explains how to run the `omni/cli` package.

## Current status

- `omni/cli` is a library package.
- `omni/cli/cmd/omni/main.go` is the executable entrypoint.
- It wires `operator.New()`, `config.DefaultOmniConfigResolver`, and `cli.Entrypoint(...).Install()`.

## Install

Using Go toolchain (recommended):

```bash
cd omni
  go install ./cli/cmd/omni
```

Or install directly from module path:

```bash
go install github.com/Shaik-Sirajuddin/memory/cli/cmd/omni@latest
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

From `omni/`:

```bash
go run ./cli/cmd/omni --help
go run ./cli/cmd/omni config get
go run ./cli/cmd/omni config get --output table
go run ./cli/cmd/omni config set --memory=true --autosync=true
go run ./cli/cmd/omni agent discover
go run ./cli/cmd/omni agent list --workspace /absolute/workspace/path
go run ./cli/cmd/omni agent init my-agent --workspace /absolute/workspace/path -p gemini --model gemini3.0-flash --interactive=true
go run ./cli/cmd/omni team-init --repo_url <git-repo-url>
go run ./cli/cmd/omni team list
go run ./cli/cmd/omni team get --id <workspace-id> --output table
go run ./cli/cmd/omni team init --repo_url <git-repo-url>
go run ./cli/cmd/omni doctor check
go run ./cli/cmd/omni doctor install
```

If installed globally:

```bash
omni --help
omni config get
omni config get --output yaml
omni agent discover
omni team-init --repo_url <git-repo-url>
omni team list
omni team get --id <workspace-id> --output json
omni team init --repo_url <git-repo-url>
omni doctor check
omni doctor install --output table
```

## Notes

- Config is stored by `DefaultOmniConfigResolver` at XDG config path:
  - `memory/omni/config.json`
- Agent commands delegate to `operator.Operator` methods.
- `doctor` table output includes:
  - `STATUS=OK` when runtime is installed
  - `STATUS=TODO` with `NEXT` action when runtime is missing
