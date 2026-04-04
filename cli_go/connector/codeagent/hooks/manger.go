package hooks

type HookType string

const (
	Webhook HookType = "webhook"
	CMD     HookType = "cmd"
)

type HookInfo struct {
	ID      HookID
	Type    HookType
	Url     *string `json:""`
	Command string
	Args    []string
	Timeout int // GPT: `json:"" jsonschema:"description:"seconds""`
}

type HookPath struct {
	Global       bool
	WorkspaceDir *string
}

type HookData struct {
	UID  string
	Path HookPath
	Info *HookInfo
}

type RegisterHookParams struct {
	Data *HookData
}

type DeleteHookParams struct {
	Global       bool
	WorkspaceDir *string
	UID          string
}

type HookManager interface {
	SupportedHooks() (*Capabilities, error)

	Register(RegisterHookParams) error
	GetRegisteredHooks() []*HookData
	// DeleteHook removes hooks matching UID
	// non existent hook return a false with non nil error
	DeleteHook(DeleteHookParams) (bool, error)
}
