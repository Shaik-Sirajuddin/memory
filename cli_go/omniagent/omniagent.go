package omniagent

import (
	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
)

type ConfigPaths struct {
	GlobalConfigDirs    []string
	WorkspaceConfigDirs []string
}

var Config ConfigPaths = ConfigPaths{
	GlobalConfigDirs: []string{
		".omni",
	},
	WorkspaceConfigDirs: []string{
		".omni",
	},
}

// Workspace level directory
const (
	AGENTS_ROOT_DIR = "/agents"
	CONFIG_FILE     = "/config.json"
)

//Example
//AGENT_DIR       = AGENTS_ROOT_DIR + "agent_name"

type AgentInfo struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	WorkspaceDir sandbox.WorkspaceDir `json:"workspace_dir"`
	// dir path for agent specific files
	MemoryDir string `json:"memory_dir"`
}

type CodeSession struct {
	Id             string           `json:"id"`
	Model          *codeagent.Model `json:"model"`
	Idx            int              `json:"idx"`
	IsActive       bool             `json:"is_active"`
	Prompts        int              `json:"prompts"`
	LastSyncPrompt int              `json:"last_sync_prompt"`
}

type PersistentMemory struct {
	// agent write memory
}

type Settings struct {
	// Default workspace
	Sandbox      *sandbox.Sandbox `json:"sandbox"`
	DefaultModel *codeagent.Model `json:"default_model"`
	hooks.Capabilities
}

type Data struct {
	Info *AgentInfo `json:"info"`
	// Current active workspace
	ActiveWorkSpace *sandbox.Sandbox `json:"active_workspace"`
	ActiveSession   *CodeSession     `json:"active_session"`
	Settings        *Settings        `json:"settings"`
	Sessions        []*CodeSession   `json:"sessions"`
}

type UpdateSettingsParams struct {
	ID       string    `json:"id"`
	Settings *Settings `json:"settings"`
}

// OmniAgent is active only in one working dir at a time
// Forking an agent is allowed
type OmniAgent interface {
	Data() *Data
	New()
	// UpdateSettings updates agent settings , the settings are reflected after completion of an ongoing command execution
	UpdateSettings(UpdateSettingsParams) error
	// SyncMemory syncronizes session memory to persistant store from current model
	SyncMemory()
	// NewCodeSession creates a new session optionally can specify a different provider , model to use
	NewCodeSession()
}

type OmniAgentEntrypoint interface {
	PreToolUse()
	PrePrompt()
	PostPrompt()
	PostToolUse()
	PreSessionStart()
	PostSessionStart()
}

type SettingsResolver interface {
	*config.OmniConfig

	Get() ([]*codeagent.Settings, error)
	// GetUnified resolves a merged config file by deduplicationg fields acorss settings
	GetUnified([]*codeagent.Settings) *Settings
	// WatchUnified watches all config files and returns the unified settings if any one of config file changes
	// applies any addition to one of config to all other configs , deletions are not propogated unless removed from omniagent.settings
	// modifying multiple files is concurrent safe
	WatchUnified(config, callback func(*Settings))
	// Apply Unified applies the settings across all codeagent configs
	ApplyUnified(*Settings) error
}

type SettingsWatcher interface {
}
