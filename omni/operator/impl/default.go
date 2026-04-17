package impl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/claude"
	claudesettings "github.com/Shaik-Sirajuddin/memory/connector/codeagent/claude/settings"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/codex"
	codexsettings "github.com/Shaik-Sirajuddin/memory/connector/codeagent/codex/settings"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/gemini"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	operator "github.com/Shaik-Sirajuddin/memory/operator"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox"
	"github.com/Shaik-Sirajuddin/memory/operator/impl/defaults"
	operatorstore "github.com/Shaik-Sirajuddin/memory/store/operator"
	"github.com/google/uuid"
)

// Provider constants mirror those in each connector package, defined here to
// avoid importing packages that currently have build errors.
const (
	providerClaude codeagent.Provider = "claude"
	providerCodex  codeagent.Provider = "codex"
	providerGemini codeagent.Provider = "gemini"
)

// DefaultOperator implements [Operator].
// agentMemory is a pluggable module: non-nil enables memory operations;
// nil disables them (e.g. when config.Features.Memory is false).
type DefaultOperator struct {
	config       config.OmniConfig
	store        operatorstore.OperatorStore
	agentMemory  operator.AgentMemory
	provisioner  sandbox.SandboxProvisioner // nil = sandboxing disabled
	newCodeAgent func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error)
}

// SwitchProvider implements [Operator].
// SwitchProvider when switching between models , if a non fill session exists matching provider , agent , session is reused , override by CleanStart
func (o *DefaultOperator) SwitchProvider(operator.SwitchProviderParams) error {
	panic("unimplemented")
}

// ResumeAgent implements [Operator].
func (o *DefaultOperator) ResumeAgent(params operator.ResumeAgentParams) error {
	workspace, err := o.resolveWorkspace(params.Workspace)
	if err != nil {
		logger.Error("ResumeAgent: workspace resolution failed", "workspace", params.Workspace, "err", err)
		return err
	}
	name := sanitizeAgentName(params.Name)
	if name == "" {
		logger.Error("ResumeAgent: validation failed", "err", "agent name is required")
		return fmt.Errorf("operator: agent name is required")
	}

	agents, err := o.store.ListAgentsByDir(workspace)
	if err != nil {
		logger.Error("ResumeAgent: list agents failed", "workspace", workspace, "err", err)
		return err
	}

	var agent *omniagent.AgentInfo
	for _, item := range agents {
		if item.Name == name {
			agent = item
			break
		}
	}
	if agent == nil {
		logger.Error("ResumeAgent: agent not found", "workspace", workspace, "name", name)
		return fmt.Errorf("operator: agent %q not found in workspace %q", name, workspace)
	}

	factory := o.newCodeAgent
	if factory == nil {
		return fmt.Errorf("operator: code agent factory is not configured")
	}
	runtime, err := factory(codeagent.Provider(operator.DefaultProvider), string(agent.WorkspaceDir), operator.DefaultModel)
	if err != nil {
		logger.Error("ResumeAgent: init code agent runtime failed", "agentID", agent.ID, "err", err)
		return fmt.Errorf("operator: init code agent runtime: %w", err)
	}
	if _, err := runtime.Resume(codeagent.ResumeSessionParams{ID: agent.ID}); err != nil {
		logger.Error("ResumeAgent: resume failed", "agentID", agent.ID, "err", err)
		return fmt.Errorf("operator: resume session for agent %q: %w", agent.ID, err)
	}
	logger.Info("ResumeAgent: completed", "agentID", agent.ID, "workspace", workspace, "name", name)
	return nil
}

// New returns an Operator with the default embedded AgentMemory module enabled.
// The memory feature is gated at runtime by OmniConfig.Features.Memory.
func New() (operator.Operator, error) {
	logger.Debug("New: initialising operator")
	s, err := operatorstore.GetOperatorStore()
	if err != nil {
		logger.Error("New: store initialisation failed", "err", err)
		return nil, fmt.Errorf("operator: init store: %w", err)
	}
	// Initialise a sandbox provisioner when a supported kind is available on the host.
	var provisioner sandbox.SandboxProvisioner
	if kinds := sandbox.HostSupportedProvisioners(); len(kinds) > 0 {
		p, perr := sandbox.NewProvisioner(kinds[0], nil, sandbox.ProvisionerOptions{})
		if perr != nil {
			logger.Warn("New: sandbox provisioner init failed", "kind", kinds[0], "err", perr)
		} else {
			provisioner = p
			logger.Info("New: sandbox provisioner ready", "kind", kinds[0])
		}
	}

	op := &DefaultOperator{
		config: config.OmniConfig{
			Features: &config.Features{Memory: true},
		},
		store:       s,
		agentMemory: operator.NewDefaultAgentMemory(),
		provisioner: provisioner,
		newCodeAgent: func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			switch provider {
			case providerClaude:
				return claude.New(workDir, model)
			case providerCodex:
				return codex.New(workDir, model)
			case providerGemini:
				return gemini.New(workDir, model)
			default:
				return nil, fmt.Errorf("operator: unknown provider %q", provider)
			}
		},
	}
	logger.Info("New: operator initialised", "memoryEnabled", op.memoryEnabled(), "sandboxEnabled", provisioner != nil)
	return op, nil
}

// memoryEnabled reports whether the agentMemory module should be invoked.
// Both the module and the feature flag must be active.
func (o *DefaultOperator) memoryEnabled() bool {
	return o.agentMemory != nil &&
		o.config.Features != nil &&
		o.config.Features.Memory
}

// DisoverCodeAgents checks which code-agent CLI binaries are present in PATH.
func (o *DefaultOperator) DisoverCodeAgents() (*operator.DisocveryResult, error) {
	logger.Debug("DisoverCodeAgents: scanning PATH for provider binaries")
	candidates := []struct {
		provider codeagent.Provider
		binary   string
	}{
		{providerGemini, "gemini"},
		{providerClaude, "claude"},
		{providerCodex, "codex"},
	}

	var providers []codeagent.Provider
	for _, c := range candidates {
		if _, err := exec.LookPath(c.binary); err == nil {
			providers = append(providers, c.provider)
			logger.Debug("DisoverCodeAgents: provider available", "provider", c.provider, "binary", c.binary)
		}
	}
	logger.Info("DisoverCodeAgents: completed", "providers", providers)
	return &operator.DisocveryResult{Providers: providers}, nil
}

// ListCodeAgents returns agents registered under the given workspace directory.
func (o *DefaultOperator) ListCodeAgents(params operator.GetCodeAgentsParams) (*operator.GetAgentsResult, error) {
	if err := params.Validate(); err != nil {
		logger.Error("ListCodeAgents: validation failed", "err", err)
		return nil, err
	}
	workspace, err := o.resolveWorkspace(params.Workspace)
	if err != nil {
		logger.Error("ListCodeAgents: workspace resolution failed", "workspace", params.Workspace, "err", err)
		return nil, err
	}
	logger.Debug("ListCodeAgents: listing agents", "workspace", workspace)
	agents, err := o.store.ListAgentsByDir(workspace)
	if err != nil {
		logger.Error("ListCodeAgents: store query failed", "workspace", workspace, "err", err)
		return nil, err
	}
	if len(agents) == 0 {
		logger.Info("ListCodeAgents: no agents found", "workspace", workspace)
		return &operator.GetAgentsResult{}, nil
	}
	logger.Info("ListCodeAgents: agent resolved", "workspace", workspace, "agentID", agents[0].ID)
	return &operator.GetAgentsResult{AgentInfo: agents[0]}, nil
}

// CreateAgent creates a new agent entry, auto-creating the workspace when needed.
// When memory is enabled, the agent's memory directory is seeded with the initial template.
func (o *DefaultOperator) CreateAgent(params operator.CreateAgentParams) error {
	if err := params.Validate(); err != nil {
		logger.Error("CreateAgent: validation failed", "err", err)
		return err
	}
	workspace, err := o.resolveWorkspace(params.Workspace)
	if err != nil {
		logger.Error("CreateAgent: workspace resolution failed", "workspace", params.Workspace, "err", err)
		return err
	}
	logger.Info("CreateAgent: start", "workspace", workspace, "name", params.Name, "provider", params.Provider, "model", params.Model, "interactive", params.Interactive, "memoryEnabled", o.memoryEnabled())

	if o.memoryEnabled() && !memoryRootExists(string(workspace)) {
		logger.Info("CreateAgent: memory root missing, initialising workspace memory", "workspace", workspace)
		if err := o.agentMemory.Init(string(workspace), ""); err != nil {
			logger.Error("CreateAgent: auto team-init failed", "workspace", workspace, "err", err)
			return fmt.Errorf("operator: create agent: init memory root: %w", err)
		}
	}

	ws, err := o.store.WorkspaceByDir(workspace)
	if err != nil {
		logger.Debug("CreateAgent: workspace missing, creating new workspace", "workspace", workspace)
		ws = &operator.TeamInfo{
			ID:           uuid.NewString(),
			Name:         filepath.Base(string(workspace)),
			WorkspaceDir: string(workspace),
		}
		if err := o.store.CreateWorkspace(ws); err != nil {
			logger.Error("CreateAgent: workspace creation failed", "workspace", workspace, "err", err)
			return fmt.Errorf("operator: create workspace: %w", err)
		}
		logger.Info("CreateAgent: workspace created", "workspaceID", ws.ID, "workspace", ws.WorkspaceDir)
	}

	agentID := uuid.NewString()
	agentName := sanitizeAgentName(params.Name)
	if agentName == "" {
		agentName = fmt.Sprintf("agent-%s", agentID[:8])
	}
	memDir := operator.AgentMemDir(string(workspace), agentName)

	agent := &omniagent.AgentInfo{
		ID:           agentID,
		Name:         agentName,
		WorkspaceDir: workspace,
		MemoryDir:    memDir,
	}
	if err := o.store.CreateAgent(agent); err != nil {
		logger.Error("CreateAgent: store insert failed", "workspaceID", ws.ID, "agentID", agentID, "err", err)
		return fmt.Errorf("operator: create agent (workspace %s): %w", ws.ID, err)
	}
	logger.Info("CreateAgent: agent stored", "workspaceID", ws.ID, "agentID", agentID, "memoryDir", memDir)

	if o.memoryEnabled() {
		if err := o.agentMemory.Create(memDir); err != nil {
			logger.Error("CreateAgent: memory seed failed", "agentID", agentID, "memoryDir", memDir, "err", err)
			return fmt.Errorf("operator: create agent: seed memory: %w", err)
		}
		logger.Info("CreateAgent: memory seeded", "agentID", agentID, "memoryDir", memDir)
	}

	if err := o.startAgentSession(agent, params.Provider, params.Model, params.Interactive); err != nil {
		logger.Error("CreateAgent: session bootstrap failed", "agentID", agentID, "provider", params.Provider, "model", params.Model, "err", err)
		return err
	}
	logger.Info("CreateAgent: completed", "workspaceID", ws.ID, "agentID", agentID)
	return nil
}

// TeamInit initialises the workspace memory root.
// Returns ErrMemoryDisabled when the memory feature is off.
func (o *DefaultOperator) TeamInit(params operator.TeamInitParams) error {
	if err := params.Validate(); err != nil {
		logger.Error("TeamInit: validation failed", "err", err)
		return err
	}
	workspace, err := o.resolveWorkspace(params.Workspace)
	if err != nil {
		logger.Error("TeamInit: workspace resolution failed", "workspace", params.Workspace, "err", err)
		return err
	}
	logger.Info("TeamInit: start", "workspace", workspace, "repoURL", params.RepoURL, "memoryEnabled", o.memoryEnabled())
	if !o.memoryEnabled() {
		logger.Error("TeamInit: memory feature disabled", "workspace", workspace)
		return operator.ErrMemoryDisabled
	}
	if err := o.agentMemory.Init(string(workspace), params.RepoURL); err != nil {
		logger.Error("TeamInit: initialisation failed", "workspace", workspace, "err", err)
		return fmt.Errorf("operator: team-init: %w", err)
	}
	if err := o.ensureWorkspaceAndGuideAgent(workspace); err != nil {
		logger.Error("TeamInit: guide agent initialisation failed", "workspace", workspace, "err", err)
		return fmt.Errorf("operator: team-init: ensure guide agent: %w", err)
	}
	logger.Info("TeamInit: completed", "workspace", workspace)
	return nil
}

// UpgradeAgent applies a newer template version to an existing agent's memory directory.
// Returns ErrMemoryDisabled when the memory feature is off.
func (o *DefaultOperator) UpgradeAgent(params operator.UpgradeAgentParams) error {
	if err := params.Validate(); err != nil {
		logger.Error("UpgradeAgent: validation failed", "err", err)
		return err
	}
	logger.Info("UpgradeAgent: start", "agentID", params.ID, "version", params.Version, "memoryEnabled", o.memoryEnabled())
	if !o.memoryEnabled() {
		logger.Error("UpgradeAgent: memory feature disabled", "agentID", params.ID)
		return operator.ErrMemoryDisabled
	}
	agent, err := o.store.GetAgent(params.ID)
	if err != nil {
		logger.Error("UpgradeAgent: agent lookup failed", "agentID", params.ID, "err", err)
		return fmt.Errorf("operator: upgrade agent %q: %w", params.ID, err)
	}
	version := params.Version
	if version == "" {
		version = operator.LatestVersion
	}
	if err := o.agentMemory.Upgrade(agent.MemoryDir, version); err != nil {
		logger.Error("UpgradeAgent: template upgrade failed", "agentID", params.ID, "version", version, "memoryDir", agent.MemoryDir, "err", err)
		return fmt.Errorf("operator: upgrade agent %q: %w", params.ID, err)
	}
	logger.Info("UpgradeAgent: completed", "agentID", params.ID, "version", version, "memoryDir", agent.MemoryDir)
	return nil
}

// ListWorkspaces returns all workspaces with their agent counts.
func (o *DefaultOperator) ListWorkspaces(_ operator.ListWorkspacesParams) (operator.ListWorkspacesResult, error) {
	logger.Debug("ListWorkspaces: loading workspaces")
	teams, err := o.store.ListWorkspaces()
	if err != nil {
		logger.Error("ListWorkspaces: store query failed", "err", err)
		return operator.ListWorkspacesResult{}, err
	}
	logger.Info("ListWorkspaces: completed", "count", len(teams))
	return operator.ListWorkspacesResult{Teams: teams}, nil
}

// GetWorkspace returns a workspace and all agents registered under it.
func (o *DefaultOperator) GetWorkspace(params operator.GetWorkSpaceParams) (operator.GetTeamResult, error) {
	if err := params.Validate(); err != nil {
		logger.Error("GetWorkspace: validation failed", "err", err)
		return operator.GetTeamResult{}, err
	}
	logger.Debug("GetWorkspace: loading workspace", "workspaceID", params.ID)
	ws, err := o.store.GetWorkspace(params.ID)
	if err != nil {
		logger.Error("GetWorkspace: workspace lookup failed", "workspaceID", params.ID, "err", err)
		return operator.GetTeamResult{}, fmt.Errorf("operator: get workspace %q: %w", params.ID, err)
	}
	agents, err := o.store.ListAgentsByDir(o.workspaceDirOf(ws))
	if err != nil {
		logger.Error("GetWorkspace: agent listing failed", "workspaceID", params.ID, "workspace", ws.WorkspaceDir, "err", err)
		return operator.GetTeamResult{}, fmt.Errorf("operator: list agents for workspace %q: %w", params.ID, err)
	}
	logger.Info("GetWorkspace: completed", "workspaceID", params.ID, "agentCount", len(agents))
	return operator.GetTeamResult{Info: ws, Agents: agents}, nil
}

// DeleteAgent removes an agent entry from the index.
// When memory is enabled, the agent's memory directory is also deleted from disk.
func (o *DefaultOperator) DeleteAgent(params operator.DeleteAgentParams) error {
	if err := params.Validate(); err != nil {
		logger.Error("DeleteAgent: validation failed", "err", err)
		return err
	}
	logger.Info("DeleteAgent: start", "agentID", params.ID, "memoryEnabled", o.memoryEnabled())
	if o.memoryEnabled() {
		agent, err := o.store.GetAgent(params.ID)
		if err == nil {
			if deleteErr := o.agentMemory.Delete(agent.MemoryDir); deleteErr != nil {
				logger.Error("DeleteAgent: memory delete failed", "agentID", params.ID, "memoryDir", agent.MemoryDir, "err", deleteErr)
			} else {
				logger.Info("DeleteAgent: memory deleted", "agentID", params.ID, "memoryDir", agent.MemoryDir)
			}
		} else {
			logger.Debug("DeleteAgent: memory delete skipped, agent lookup failed", "agentID", params.ID, "err", err)
		}
	}
	if err := o.store.DeleteAgent(params.ID); err != nil {
		logger.Error("DeleteAgent: store delete failed", "agentID", params.ID, "err", err)
		return err
	}
	logger.Info("DeleteAgent: completed", "agentID", params.ID)
	return nil
}

// workspaceDirOf converts a TeamInfo directory string to a WorkspaceDir.
func (o *DefaultOperator) workspaceDirOf(ws *operator.TeamInfo) sandbox.WorkspaceDir {
	return sandbox.WorkspaceDir(ws.WorkspaceDir)
}

func (o *DefaultOperator) resolveWorkspace(in sandbox.WorkspaceDir) (sandbox.WorkspaceDir, error) {
	if in != "" {
		return in, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("operator: resolve workspace: getwd: %w", err)
	}
	if resolved, ok := findWorkspaceFromMemory(cwd); ok {
		logger.Debug("resolveWorkspace: resolved from ancestor memory root", "cwd", cwd, "workspace", resolved)
		return sandbox.WorkspaceDir(resolved), nil
	}
	logger.Debug("resolveWorkspace: defaulting to cwd", "cwd", cwd)
	return sandbox.WorkspaceDir(cwd), nil
}

func findWorkspaceFromMemory(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if memoryRootExists(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func memoryRootExists(workspace string) bool {
	info, err := os.Stat(filepath.Join(workspace, operator.MemoryDirName))
	return err == nil && info.IsDir()
}

func sanitizeAgentName(name string) string {
	return strings.TrimSpace(name)
}

func (o *DefaultOperator) startAgentSession(agent *omniagent.AgentInfo, provider codeagent.Provider, model string, interactive bool) error {
	if provider == "" {
		provider = codeagent.Provider(operator.DefaultProvider)
	}
	if model == "" {
		model = operator.DefaultModel
	}

	factory := o.newCodeAgent
	if factory == nil {
		return fmt.Errorf("operator: code agent factory is not configured")
	}

	runtime, err := factory(provider, string(agent.WorkspaceDir), model)
	if err != nil {
		return fmt.Errorf("operator: init code agent runtime: %w", err)
	}

	// Provision a sandbox runtime for this session when the provisioner is available.
	// The default sandbox config grants the agent access to the provider binary directories.
	var sbxRuntime *sandbox.SandboxRuntime
	if o.provisioner != nil {
		cfg := defaults.SandboxConfig(provider)
		rt, sbxErr := o.provisioner.Create(sandbox.CreateSandboxParams{
			ID:     agent.ID,
			Config: cfg,
		})
		if sbxErr != nil {
			logger.Warn("startAgentSession: sandbox provision failed", "agentID", agent.ID, "err", sbxErr)
		} else {
			sbxRuntime = &rt
			logger.Debug("startAgentSession: sandbox runtime provisioned", "agentID", agent.ID, "provider", provider)
		}
	}

	createResult, err := runtime.Create(codeagent.CreateSessionParams{
		ID:      agent.ID,
		Name:    agent.Name,
		Model:   model,
		WorkDir: string(agent.WorkspaceDir),
		RunTime: sbxRuntime,
	})
	if err != nil {
		return fmt.Errorf("operator: create session for agent %q: %w", agent.ID, err)
	}

	sessionID := createResult.ID
	if sessionID == "" {
		sessionID = agent.ID
	}
	logger.Info("startAgentSession: session created", "agentID", agent.ID, "provider", provider, "sessionID", sessionID, "interactive", interactive)

	if !interactive {
		return nil
	}

	// operator holds codeagents , it sends a callback to codeagent on sandbox change
	// codeagent becomes passive component , where it returns only data
	if _, err := runtime.Resume(codeagent.ResumeSessionParams{ID: sessionID}); err != nil {
		return fmt.Errorf("operator: resume session for agent %q: %w", agent.ID, err)
	}
	logger.Info("startAgentSession: interactive session resumed", "agentID", agent.ID, "provider", provider, "sessionID", sessionID)
	return nil
}

func (o *DefaultOperator) ensureWorkspaceAndGuideAgent(workspace sandbox.WorkspaceDir) error {
	ws, err := o.store.WorkspaceByDir(workspace)
	if err != nil {
		ws = &operator.TeamInfo{
			ID:           uuid.NewString(),
			Name:         filepath.Base(string(workspace)),
			WorkspaceDir: string(workspace),
		}
		if err := o.store.CreateWorkspace(ws); err != nil {
			return err
		}
	}

	agents, err := o.store.ListAgentsByDir(workspace)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.Name == "guide" {
			return nil
		}
	}

	guide := &omniagent.AgentInfo{
		ID:           uuid.NewString(),
		Name:         "guide",
		WorkspaceDir: workspace,
		MemoryDir:    operator.AgentMemDir(string(workspace), "guide"),
	}
	if err := o.store.CreateAgent(guide); err != nil {
		return err
	}
	if o.memoryEnabled() {
		if err := o.agentMemory.Create(guide.MemoryDir); err != nil {
			return err
		}
	}
	return nil
}

// GetCodeAgentResolver returns the SettingsResolver for the given provider.
// Claude and Codex use their settings sub-packages directly.
// Gemini has no exported settings constructor and returns ErrResolverUnavailable.
func (o *DefaultOperator) GetCodeAgentResolver(agent codeagent.Provider) (*codeagent.SettingsResolver, error) {
	logger.Debug("GetCodeAgentResolver: resolving provider", "provider", agent)
	var r codeagent.SettingsResolver
	switch agent {
	case providerClaude:
		r = claudesettings.New(providerClaude)
	case providerCodex:
		r = codexsettings.New(providerCodex)
	case providerGemini:
		logger.Error("GetCodeAgentResolver: resolver unavailable", "provider", providerGemini)
		return nil, fmt.Errorf("operator: %w: %s", operator.ErrResolverUnavailable, providerGemini)
	default:
		logger.Error("GetCodeAgentResolver: unknown provider", "provider", agent)
		return nil, fmt.Errorf("operator: unknown provider %q", agent)
	}
	logger.Info("GetCodeAgentResolver: resolver ready", "provider", agent)
	return &r, nil
}

