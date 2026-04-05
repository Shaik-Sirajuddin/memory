package operator

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/claude"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/codex"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/gemini"
	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	"github.com/adrg/xdg"
	"github.com/google/uuid"
)

type sqlOperator struct {
	store OperatorStore
}

// New returns an Operator backed by the shared omniagent SQLite database.
// It uses GetOperatorStore as the factory for the data layer.
func New() (Operator, error) {
	s, err := GetOperatorStore()
	if err != nil {
		return nil, fmt.Errorf("operator: init store: %w", err)
	}
	return &sqlOperator{store: s}, nil
}

// DisoverCodeAgents checks which code-agent CLI binaries are present in PATH.
func (o *sqlOperator) DisoverCodeAgents() (*DisocveryResult, error) {
	candidates := []struct {
		provider codeagent.Provider
		binary   string
	}{
		{gemini.Gemini, "gemini"},
		{claude.Claude, "claude"},
		{codex.Codex, "codex"},
	}

	var providers []codeagent.Provider
	for _, c := range candidates {
		if _, err := exec.LookPath(c.binary); err == nil {
			providers = append(providers, c.provider)
		}
	}
	return &DisocveryResult{Providers: providers}, nil
}

// ListCodeAgents returns agents registered under the given workspace directory.
func (o *sqlOperator) ListCodeAgents(params GetCodeAgentsParams) (*GetAgentsResult, error) {
	agents, err := o.store.ListAgentsByDir(params.Workspace)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return &GetAgentsResult{}, nil
	}
	return &GetAgentsResult{AgentInfo: agents[0]}, nil
}

// CreateAgent creates a new agent entry and, when no workspace exists for the
// directory yet, creates one first.
func (o *sqlOperator) CreateAgent(params CreateAgentParams) error {
	ws, err := o.store.WorkspaceByDir(params.Workspace)
	if err != nil {
		// No existing workspace — create one.
		ws = &TeamInfo{
			ID:           uuid.NewString(),
			Name:         string(params.Workspace),
			WorkspaceDir: string(params.Workspace),
		}
		if err := o.store.CreateWorkspace(ws); err != nil {
			return fmt.Errorf("operator: create workspace: %w", err)
		}
	}

	agentID := uuid.NewString()
	memDir := filepath.Join(xdg.DataHome, "memory", "agents", agentID)

	agent := &omniagent.AgentInfo{
		ID:           agentID,
		Name:         fmt.Sprintf("agent-%s", agentID[:8]),
		WorkspaceDir: params.Workspace,
		MemoryDir:    memDir,
	}
	if err := o.store.CreateAgent(agent); err != nil {
		return fmt.Errorf("operator: create agent (workspace %s): %w", ws.ID, err)
	}
	return nil
}

// ListWorkspaces returns all workspaces with their agent counts.
func (o *sqlOperator) ListWorkspaces(_ ListWorkspacesParams) (ListWorkspacesResult, error) {
	teams, err := o.store.ListWorkspaces()
	if err != nil {
		return ListWorkspacesResult{}, err
	}
	return ListWorkspacesResult{Teams: teams}, nil
}

// GetWorkspace returns a workspace and all agents registered under it.
func (o *sqlOperator) GetWorkspace(params GetWorkSpaceParams) (GetTeamResult, error) {
	ws, err := o.store.GetWorkspace(params.ID)
	if err != nil {
		return GetTeamResult{}, fmt.Errorf("operator: get workspace %q: %w", params.ID, err)
	}

	agents, err := o.store.ListAgentsByDir(o.workspaceDirOf(ws))
	if err != nil {
		return GetTeamResult{}, fmt.Errorf("operator: list agents for workspace %q: %w", params.ID, err)
	}
	return GetTeamResult{Info: ws, Agents: agents}, nil
}

// DeleteAgent removes an agent entry from the index; memory files are retained.
func (o *sqlOperator) DeleteAgent(params DeleteAgentParams) error {
	return o.store.DeleteAgent(params.ID)
}

// workspaceDirOf converts a TeamInfo's directory string to a WorkspaceDir.
func (o *sqlOperator) workspaceDirOf(ws *TeamInfo) sandbox.WorkspaceDir {
	return sandbox.WorkspaceDir(ws.WorkspaceDir)
}
