package operator

import (
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
)

const (
	DefaultProvider        = codeagent.Gemini
	DefaultModel           = "gemini3.0-flash"
	DefaultProviderVersion = "1.0"
)

type GetCodeAgentsParams struct {
	Workspace string
}

type GetAgentsResult struct {
	AgentInfo *omniagent.CodeAgentInfo
}

type CreateAgentParams struct {
	Workspace   string
	Dir         string // default pwd
	Interactive bool   //launch after create default true
}

type DeleteAgentParams struct {
	ID string
}

type TeamInfo struct {
	ID         string
	Name       string
	WorkingDir string
	Agents     int
}
type ListWorkspacesParams struct {
}
type ListWorkspacesResult struct {
	Teams []*TeamInfo
}

type GetWorkSpaceParams struct {
	ID string
}

// type TeamDefaults struct {
// 	AgentDefaults *codeagent.AgentDefaults
// 	AgentData     *codeagent.Data
// }

type GetTeamResult struct {
	Info   *TeamInfo
	Agents []*omniagent.CodeAgentInfo
}
type ForkAgentParams struct {
	ID       string
	Settings *omniagent.Settings
}

type Operator interface {
	ListCodeAgents(params GetCodeAgentsParams) (*GetAgentsResult, error)
	CreateAgent(params CreateAgentParams) error
	ListWorkspaces(params ListWorkspacesParams) (ListWorkspacesResult, error)
	GetWorkspace(params GetWorkSpaceParams) (GetTeamResult, error)
	// DeleteAgent from index , memory is retained
	DeleteAgent(params DeleteAgentParams) error
	// ForkAgent(params)
}
