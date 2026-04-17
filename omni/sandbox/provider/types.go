package provider

import "io"

type WorkspaceDir string

const (
	Default WorkspaceDir = "_default"
)

type MountConfig struct {
	AccessDirs  []string // regex supported
	BlockedDirs []string // regex supported
}

type Policy struct {
	Dir      WorkspaceDir
	FSPolicy FSPolicy
	Config   MountConfig
}

type Config struct {
	WorkSpacePolicy *Policy
	AgentPolicy     *Policy
}

type State struct {
	PID    string
	Active bool
}

type Data struct {
	ID          string
	Application string
	CreatedAt   string
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
	Executable  string
	WorkDir     string
	ProfileName string
	ExtraArgs   []string
	GlobalArgs  []string
	RuntimeRoot string
	Store       SandboxStore
}

type GetSandboxParams struct {
	PID    *string
	Name   *string
	Active bool
}

type ListSandboxParams struct {
	Active bool
}

type CreateSandboxParams struct {
	ID     string
	Config *Config
}

type ExecutionResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
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

type Info struct {
	Application string
}

type SandboxStore interface {
	Info() Info
	Create(*Sandbox) error
	Update(*Sandbox) error
	Get(*GetSandboxParams) (*Sandbox, error)
	List() ([]*Sandbox, error)
}

type Store = SandboxStore
