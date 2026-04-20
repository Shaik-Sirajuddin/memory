package provider

import "io"

type WorkspaceDir string

const (
	Default WorkspaceDir = "_default"
)

type MountConfig struct {
	AccessDirs  []string `json:"access_dirs,omitempty"`  // regex supported
	BlockedDirs []string `json:"blocked_dirs,omitempty"` // regex supported
}

type Policy struct {
	Dir      WorkspaceDir `json:"dir,omitempty"`
	FSPolicy FSPolicy     `json:"fs_policy,omitempty"`
	Config   MountConfig  `json:"config,omitempty"`
}

type Config struct {
	WorkSpacePolicy *Policy `json:"workspace_policy,omitempty"`
	AgentPolicy     *Policy `json:"agent_policy,omitempty"`
}

type State struct {
	PID    string `json:"pid,omitempty"`
	Active bool   `json:"active,omitempty"`
}

type Data struct {
	ID          string `json:"id,omitempty"`
	ConfigDir   string `json:"config_dir,omitempty"`
	Application string `json:"application,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type Sandbox struct {
	*Config
	*State
	*Data
}

type FSPolicy string

type AgentFSPolicy FSPolicy

const (
	AllPermissiveRead AgentFSPolicy = "all-permissive"
	PermissiveRead    AgentFSPolicy = "permissive-read"
	NonDependent      AgentFSPolicy = "no-external"
	Inherit           AgentFSPolicy = "inherit"

	DefaultPolicy AgentFSPolicy = PermissiveRead
)

type ProvisionerKind string

const (
	ProvisionerGVisor     ProvisionerKind = "gvisor"
	ProvisionerBubblewrap ProvisionerKind = "bubblewrap"
	ProvisionerSeatbelt   ProvisionerKind = "seatbelt"
)

type ProvisionerOptions struct {
	Executable   string           `json:"executable,omitempty"`
	WorkDir      string           `json:"work_dir,omitempty"`
	ArtifactsDir string           `json:"artifacts_dir,omitempty"`
	ProfileName  string           `json:"profile_name,omitempty"`
	ExtraArgs    []string         `json:"extra_args,omitempty"`
	GlobalArgs   []string         `json:"global_args,omitempty"`
	RuntimeRoot  string           `json:"runtime_root,omitempty"`
	ConfigParser ConfigFileParser `json:"-"`
	Store        SandboxStore     `json:"-"`
}

type GetSandboxParams struct {
	PID    *string `json:"pid,omitempty"`
	Name   *string `json:"name,omitempty"`
	Active bool    `json:"active,omitempty"`
}

type ListSandboxParams struct {
	Active bool `json:"active,omitempty"`
}

type CreateSandboxParams struct {
	ID        string  `json:"id,omitempty"`
	ConfigDir string  `json:"config_dir,omitempty"`
	Config    *Config `json:"config,omitempty"`
}

type UpdateSandboxParams struct {
	ID     string  `json:"id,omitempty"`
	Config *Config `json:"config,omitempty"`
}

type ParsedSandboxConfig struct {
	AllowWrite  bool     `json:"allow_write,omitempty"`
	AccessDirs  []string `json:"access_dirs,omitempty"`
	BlockedDirs []string `json:"blocked_dirs,omitempty"`
}

type ExecutionResult struct {
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

type SandboxProcess interface {
	PID() int
	Stdout() io.Reader
	Stderr() io.Reader
	Wait() (*ExecutionResult, error)
	Kill() error
}

type SandboxRuntime interface {
	Sandbox() *Sandbox
	Command(command string, args []string) error
	Execute(command string, args []string) error
	Capture(command string, args []string) (*ExecutionResult, error)
	Start(command string, args []string) (SandboxProcess, error)
	Sync(config *Config) error
}

type SandboxProvisioner interface {
	Create(params CreateSandboxParams) (SandboxRuntime, error)
	List(ListSandboxParams) ([]SandboxRuntime, error)
	GetSandbox(params *GetSandboxParams) (SandboxRuntime, error)
}

type SandboxDirProvisioner interface {
	CreateDir(path string) error
	ListDirs(path string) ([]string, error)
}

type SandboxUpdateProvisioner interface {
	UpdateSandbox(params *UpdateSandboxParams) (SandboxRuntime, error)
}

type SandboxConfigParser interface {
	Parse(config *Config) (*ParsedSandboxConfig, error)
}




type ConfigFileParser interface {
	Load(filePath string) (*Config, error)
	Validate(config *Config) error
	Save(config *Config, filePath string) error
}

type Info struct {
	Application string `json:"application,omitempty"`
}

type SandboxStore interface {
	Info() Info
	Create(*Sandbox) error
	Update(*Sandbox) error
	Get(*GetSandboxParams) (*Sandbox, error)
	List() ([]*Sandbox, error)
}

type Store = SandboxStore
