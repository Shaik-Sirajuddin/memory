package provider

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

type SandboxProvisioner interface {
	Create(params CreateSandboxParams) (*Sandbox, error)
	Command(pid, command string, args []string) error
	Execute(pid, command string, args []string) error
	Sync(config *Config) error
	List(ListSandboxParams) ([]*Sandbox, error)
	GetSandbox(params *GetSandboxParams) (*Sandbox, error)
}

type Info struct {
	Application string
}

type Store interface {
	Info() Info
	Create(*Sandbox) error
	Update(*Sandbox) error
	List() ([]*Sandbox, error)
}
