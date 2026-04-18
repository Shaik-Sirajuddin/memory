package operator

import (
	"fmt"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)
	
const (
	DefaultProvider        = "claude"
	DefaultModel           = "claude-sonnet-4-6"
	DefaultProviderVersion = "1.0"
)

type GetCodeAgentsParams struct {
	Workspace sandbox.WorkspaceDir `json:"workspace,omitempty"`
}

type GetAgentsResult struct {
	Agents []*omniagent.AgentInfo `json:"agents"`
}

type CreateAgentParams struct {
	Workspace          sandbox.WorkspaceDir `json:"workspace,omitempty"`
	Name               string               `json:"name,omitempty"`
	Provider           codeagent.Provider   `json:"provider,omitempty"`
	Model              string               `json:"model,omitempty"`
	AllowGeneratedName bool                 `json:"allow_generated_name,omitempty"`
	Interactive        bool                 `json:"interactive"` // launch after create; default true
}

type ResumeAgentParams struct {
	Workspace sandbox.WorkspaceDir `json:"workspace,omitempty"`
	Name      string               `json:"name,omitempty"`
}

type DeleteAgentParams struct {
	ID string `json:"id"`
}

type TeamInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	WorkspaceDir string `json:"workspace_dir"`
	Agents       int    `json:"agents"`
}

type ListWorkspacesParams struct{}

type ListWorkspacesResult struct {
	Teams []*TeamInfo `json:"teams"`
}

type GetWorkSpaceParams struct {
	ID string `json:"id"`
}

type TeamInitParams struct {
	Workspace sandbox.WorkspaceDir `json:"workspace,omitempty"`
	// RepoURL is optional. When set the memory dir is tracked as a git submodule.
	// When empty an empty git repository is initialised inside the memory dir.
	RepoURL string `json:"repo_url,omitempty"`
}

type UpgradeAgentParams struct {
	ID string `json:"id"`
	// Version to upgrade to (e.g. "v2"). Empty means upgrade to the latest embedded template.
	Version string `json:"version,omitempty"`
}

// type TeamDefaults struct {
// 	AgentDefaults *codeagent.AgentDefaults
// 	AgentData     *codeagent.Data
// }

type GetTeamResult struct {
	Info   *TeamInfo              `json:"info"`
	Agents []*omniagent.AgentInfo `json:"agents"`
}

type ForkAgentParams struct {
	ID       string              `json:"id"`
	Settings *omniagent.Settings `json:"settings"`
}

type SwitchProviderParams struct {
	ID         string             `json:"id"`
	CleanStart bool               `json:"clean_start,omitempty"`
	Provider   codeagent.Provider `json:"provider,omitempty"`
}

// ProviderModels pairs a provider with its available model IDs.
type ProviderModels struct {
	Provider codeagent.Provider `json:"provider"`
	Models   []string           `json:"models"`
}

type DisocveryResult struct {
	Providers []codeagent.Provider `json:"providers"`
	Models    []ProviderModels     `json:"models"`
}

func (p GetCodeAgentsParams) Validate() error {
	return nil
}

func (p CreateAgentParams) Validate() error {
	if p.Name == "" && !p.AllowGeneratedName {
		return fmt.Errorf("operator: agent name is required unless generated names are enabled")
	}
	return nil
}

func (p DeleteAgentParams) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("operator: agent id is required")
	}
	return nil
}

func (p GetWorkSpaceParams) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("operator: workspace id is required")
	}
	return nil
}

func (p TeamInitParams) Validate() error {
	return nil
}

func (p UpgradeAgentParams) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("operator: agent id is required")
	}
	return nil
}

// ErrMemoryDisabled is returned when a memory operation is attempted but the feature is off.
var ErrMemoryDisabled = fmt.Errorf("operator: memory feature disabled")

// ErrResolverUnavailable is returned when a provider has no exported settings resolver.
var ErrResolverUnavailable = fmt.Errorf("operator: settings resolver unavailable for provider")

// Operator manages the state of default agents
// provisioning of new agent happens through operator
type Operator interface {
	// DisoverCodeAgents performs discover of available agents in local pc
	// GPT : DisoverCodeAgents calls codeagents info checks to filter available agents
	DisoverCodeAgents() (*DisocveryResult, error)
	ListCodeAgents(params GetCodeAgentsParams) (*GetAgentsResult, error)

	// Createagent creates an agent and creates a team when no agents exist in the workspace
	// else the agent is added to existing team
	CreateAgent(params CreateAgentParams) error

	ListWorkspaces(params ListWorkspacesParams) (ListWorkspacesResult, error)
	GetWorkspace(params GetWorkSpaceParams) (GetTeamResult, error)
	// DeleteAgent from index , memory is retained
	DeleteAgent(params DeleteAgentParams) error
	// ForkAgent(params)
	GetCodeAgentResolver(agent codeagent.Provider) (*codeagent.SettingsResolver, error)

	// TeamInit initialises the memory folder for a workspace. It runs git submodule add
	// when RepoURL is set, otherwise initialises a bare local git repo inside memory/.
	// Memory is seeded with the current template regardless of the git strategy.
	TeamInit(params TeamInitParams) error

	// UpgradeAgent applies a newer version template to an existing agent's memory dir.
	UpgradeAgent(params UpgradeAgentParams) error

	// Resume agent launches the codeagent process interactivesly
	// continues the last session
	// Launches a new session when no previous session exists
	ResumeAgent(ResumeAgentParams) error

	// SwtichProvider switches the underlying model of current agent
	// Retaining memories from the summaries generated
	SwitchProvider(SwitchProviderParams) error
}
