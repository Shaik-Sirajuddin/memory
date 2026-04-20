## sandbox

> Do not read implementation files unless explicitly asked to.

### Data Structs
- `provider/types.go` — `WorkspaceDir`, `MountConfig`, `Policy`, `Config`, `State`, `Data`, `Sandbox`, `ProvisionerOptions`, `GetSandboxParams`, `ListSandboxParams`, `CreateSandboxParams`, `UpdateSandboxParams`, `ParsedSandboxConfig`, `ExecutionResult`, `Info`
- `provider/shared.go` — `ProvisionerState`
- `doctor.go` — `HealthStatus`

### Interfaces
- `provider/types.go` — `SandboxRuntime`, `SandboxProcess`, `SandboxProvisioner`, `SandboxUpdateProvisioner`, `SandboxDirProvisioner`, `SandboxConfigParser`, `Store`
- `sandbox.go` — `ConfigParser`
- `provider/gvisor/default.go` — `ConigTransformer`
- `store/store.go` — `SandboxStore`

### Factory Functions
- `sandbox.go` — `NewProvisioner(kind ProvisionerKind, sbx *Sandbox, opts ProvisionerOptions) (SandboxProvisioner, error)` — selects `gvisor`, `bubblewrap`, or `seatbelt`
- `sandbox.go` — `SupportedProvisioners(goos string) []ProvisionerKind` — host capability map
- `sandbox.go` — `HostSupportedProvisioners() []ProvisionerKind` — runtime.GOOS wrapper
- `sandbox.go` — `NewConfigParser() ConfigParser` — default koanf-backed config parser factory
- `doctor.go` — `NewDoctor() *Doctor` — sandbox doctor for runtime health/install checks
- `store/default.go` — `GetSandboxStore(application string) (SandboxStore, error)` — singleton YAML+SQLite-backed store factory
- `provider/gvisor/default.go` — `New(sbx *provider.Sandbox, opts provider.ProvisionerOptions) (*Provisioner, error)`
- `provider/bubblewrap/default.go` — `New(sbx *provider.Sandbox, opts provider.ProvisionerOptions) (*Provisioner, error)`
- `provider/seatbelt/default.go` — `New(sbx *provider.Sandbox, opts provider.ProvisionerOptions) (*Provisioner, error)`
- `log/log.go` — `NewLogger(provider string) *slog.Logger` — sandbox-scoped logger constructor backed by `omni/log`

### Modules
- `doctor.go` — `Doctor.Health()` detects runtime installation (`runsc` on Linux, `sandbox-exec` on macOS)
- `doctor.go` — `Doctor.Install()` auto-runs `scripts/install-sandbox-runtime.sh` for Linux when `runsc` is missing
- `sandbox.go` — `Load(filePath string)`, `Validate(config *Config)`, `Save(config *Config, filePath string)` top-level config parser helpers
- `default.go` — default `ConfigParser` implementation using koanf for file load/unmarshal and JSON save

### Shared Helpers
- `provider/shared.go` — `NewProvisionerState() ProvisionerState` — in-memory lifecycle state holder
- `provider/shared.go` — `CloneConfig`, `ClonePolicy`, `CloneSandbox` — deep-copy helpers for sandbox state/config
- `provider/shared.go` — `SandboxAllowsWrite`, `SandboxAccessDirs`, `SandboxBlockedDirs`, `UniqueCleaned` — policy normalization helpers
- `provider/runtime.go` — `RunCaptured`, `StartCaptured` — provider-neutral captured execution and process start helpers
- `common/templates.go` — `DefaultAgentTemplate`, `EnsureCommonConfig`, `SyncCommonConfig`, `WriteProviderTemplate` — ConfigDir/common template provisioning helpers

### Providers
- `provider/gvisor/default.go` — gVisor `runsc`-backed lifecycle implementation
- `provider/bubblewrap/default.go` — Linux `bwrap`-backed lifecycle implementation
- `provider/seatbelt/default.go` — macOS `sandbox-exec` / Seatbelt-backed lifecycle implementation
- Each provider now returns a long-lived `SandboxRuntime`; execution methods live on the runtime instead of the provisioner
- `provider/gvisor/default.go` and `provider/seatbelt/default.go` implement optional parser-driven `UpdateSandbox` and directory methods (`CreateDir`, `ListDirs`)

### Persistence
- `store/default.go` — stores runtime metadata in SQLite and sandbox config in YAML files
- `store/database/schema.sql` — `sandboxes` table schema

### Notes
- Root package `omni/sandbox` is a facade over `provider/*`.
- Shared types are imported throughout the codebase via `github.com/Shaik-Sirajuddin/memory/sandbox/provider`.
- Linux support maps to `gvisor` in `SupportedProvisioners`.
- macOS support maps to `seatbelt`.
- Windows is expected to follow Linux semantics through WSL2 rather than a separate native provider.
