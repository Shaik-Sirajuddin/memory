package operator

import (
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/gemini"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
)

const (
	DefaultProvider        = gemini.Gemini
	DefaultModel           = "gemini3.0-flash"
	DefaultProviderVersion = "1.0"
)

type GetCodeAgentsParams struct {
	Workspace sandbox.WorkspaceDir `json:"workspace"`
}

type GetAgentsResult struct {
	AgentInfo *omniagent.AgentInfo `json:"agent_info"`
}

type CreateAgentParams struct {
	Workspace   sandbox.WorkspaceDir `json:"workspace"`
	Interactive bool                 `json:"interactive"` // launch after create; default true
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

// type TeamDefaults struct {
// 	AgentDefaults *codeagent.AgentDefaults
// 	AgentData     *codeagent.Data
// }

type GetTeamResult struct {
	Info   *TeamInfo            `json:"info"`
	Agents []*omniagent.AgentInfo `json:"agents"`
}

type ForkAgentParams struct {
	ID       string             `json:"id"`
	Settings *omniagent.Settings `json:"settings"`
}

type DisocveryResult struct {
	Providers []codeagent.Provider `json:"providers"`
}

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
}
