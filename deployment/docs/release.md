# Release Guide

## Prerequisites

- Go toolchain on `PATH`
- [goreleaser](https://goreleaser.com) installed:
  ```bash
  go install github.com/goreleaser/goreleaser/v2@latest
  ```
- `gh` CLI authenticated (`gh auth login`) **or** `GITHUB_TOKEN` env var set
- Must be on the `main` branch with a version tag pushed

## Versioning

GoReleaser reads the version from a **git tag** — there is no version field in any
config file.

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

## Running a release

From `omni/` (or repo root):

```bash
# from omni/
make release

# equivalent direct call from repo root
goreleaser release --clean
```

### Makefile targets

| Target | Command | Description |
|--------|---------|-------------|
| `make release` | `goreleaser release --clean` | Full release — builds, archives, publishes to GitHub |
| `make snapshot` | `goreleaser release --snapshot --clean` | Local dry-run — no tag needed, artifacts in `dist/` |
| `make release-check` | `goreleaser check` | Validate `.goreleaser.yaml` syntax |

## What goreleaser does

1. Runs `go mod tidy` on `omni/` and `svc/cmd/`
2. Cross-compiles two binaries for each Linux target (`linux/amd64`, `linux/arm64`):
   - `omni` — CLI (`omni/cli/cmd/omni/`)
   - `omni-server` — in-process supervisor (`svc/cmd/`); embeds ptydaemon and hook-operator as goroutines
3. Injects the git tag as `main.Version` via `-ldflags`
4. Packages each target as a tarball:
   ```
   omni-<version>-<os>-<arch>.tar.gz
     omni-<version>-<os>-<arch>/
       omni
       omni-server   ← svc/cmd: runs ptydaemon + hook-operator in-process
       deployment/setup.sh
   ```
5. Generates a checksums file
6. Creates a GitHub release and uploads all tarballs + checksums

Artifacts are written to `dist/` and cleaned on each run.

## Snapshot (local dry-run)

```bash
make snapshot
# artifacts in dist/ — nothing published
```

No git tag required for a snapshot. The version is set to `<last-tag>-snapshot`.

## Config file

`.goreleaser.yaml` at the repo root. Edit it to:
- Add Darwin/Windows targets (stubbed with comments)
- Add extra files to tarballs (`archives[].files`)
- Change the GitHub repo (`release.github`)

## On-machine install from tarball

```bash
tar -xzf omni-v1.2.3-linux-amd64.tar.gz
sudo bash omni-v1.2.3-linux-amd64/deployment/setup.sh
```
