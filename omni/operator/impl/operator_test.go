package impl

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
	operator "github.com/Shaik-Sirajuddin/memory/operator"
	operatorstore "github.com/Shaik-Sirajuddin/memory/store/operator"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const testAgentSchema = `
CREATE TABLE IF NOT EXISTS agents (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    workspace_dir TEXT NOT NULL,
    memory_dir    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_settings (
    agent_id               TEXT PRIMARY KEY,
    sandbox                TEXT NOT NULL DEFAULT '{}',
    default_model_provider TEXT NOT NULL DEFAULT '',
    default_model_name     TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);`

const testWorkspaceSchema = `
CREATE TABLE IF NOT EXISTS workspaces (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    workspace_dir TEXT NOT NULL UNIQUE
);`

func TestStoreSchemaIncludesRequiredTables(t *testing.T) {
	db := newTestDB(t)

	assert.True(t, tableExists(t, db, "agents"), "Agents table should exist in the temp sqlite store")
	assert.True(t, tableExists(t, db, "agent_settings"), "Agent settings table should exist in the temp sqlite store")
	assert.True(t, tableExists(t, db, "workspaces"), "Workspaces table should exist in the temp sqlite store")
}

func TestCreateAgent(t *testing.T) {
	t.Run("MemoryEnabledCreatesTemplateFiles", func(t *testing.T) {
		op := newTestOperator(t, true)
		workspace := sandbox.WorkspaceDir(t.TempDir())

		err := op.CreateAgent(operator.CreateAgentParams{Workspace: workspace, Name: "operator-a", Interactive: true})
		require.NoError(t, err, "CreateAgent should succeed when memory is enabled")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{Workspace: workspace})
		require.NoError(t, err, "ListCodeAgents should resolve the created agent")
		require.NotNil(t, result.AgentInfo, "Created agent should be stored")

		assert.DirExists(t, result.AgentInfo.MemoryDir, "Agent memory directory should exist")
		assert.Equal(t, "operator-a", result.AgentInfo.Name, "Requested agent name should be preserved")
		assert.Equal(t, filepath.Join(string(workspace), "memory", result.AgentInfo.Name), result.AgentInfo.MemoryDir, "Agent memory directory should follow the new layout")
		_, memoryStatErr := os.Stat(filepath.Join(result.AgentInfo.MemoryDir, "entry", "data", "memory.yaml"))
		assert.ErrorIs(t, memoryStatErr, os.ErrNotExist, "Per-agent memory.yaml should not be created")
		_, semanticsStatErr := os.Stat(filepath.Join(result.AgentInfo.MemoryDir, "entry", "data", "semantics.yaml"))
		assert.ErrorIs(t, semanticsStatErr, os.ErrNotExist, "Per-agent semantics.yaml should not be created")
		assert.DirExists(t, filepath.Join(result.AgentInfo.MemoryDir, "generated"), "Generated directory should be created")
		assert.DirExists(t, filepath.Join(result.AgentInfo.MemoryDir, "state"), "State directory should be created")
		assert.FileExists(t, filepath.Join(string(workspace), "memory", "memory.yaml"), "Workspace memory.yaml should be created instead")
		assert.FileExists(t, filepath.Join(string(workspace), "agent_"+result.AgentInfo.Name+".md"), "Workspace should include the agent entry markdown")

		workspaces, err := op.ListWorkspaces(operator.ListWorkspacesParams{})
		require.NoError(t, err, "ListWorkspaces should succeed after agent creation")
		require.Len(t, workspaces.Teams, 1, "Exactly one workspace should be created")
		assert.Equal(t, 1, workspaces.Teams[0].Agents, "Workspace agent count should reflect the inserted agent")

		workspaceData, err := op.GetWorkspace(operator.GetWorkSpaceParams{ID: workspaces.Teams[0].ID})
		require.NoError(t, err, "GetWorkspace should return the created workspace")
		require.NotNil(t, workspaceData.Info, "Workspace info should be populated")
		assert.Len(t, workspaceData.Agents, 1, "Workspace lookup should include the created agent")
	})

	t.Run("MemoryDisabledSkipsTemplateFiles", func(t *testing.T) {
		op := newTestOperator(t, false)
		workspace := sandbox.WorkspaceDir(t.TempDir())

		err := op.CreateAgent(operator.CreateAgentParams{Workspace: workspace, Name: "operator-b"})
		require.NoError(t, err, "CreateAgent should still persist the agent when memory is disabled")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{Workspace: workspace})
		require.NoError(t, err, "ListCodeAgents should resolve the stored agent")
		require.NotNil(t, result.AgentInfo, "Created agent should be stored")

		_, statErr := os.Stat(result.AgentInfo.MemoryDir)
		assert.ErrorIs(t, statErr, os.ErrNotExist, "Agent memory directory should not be created when memory is disabled")
	})

	t.Run("AutoInitialisesMemoryInCurrentWorkspaceWhenNoMemoryExists", func(t *testing.T) {
		op := newTestOperator(t, true)
		cwd := t.TempDir()

		prev, err := os.Getwd()
		require.NoError(t, err, "Current working directory lookup should succeed")
		require.NoError(t, os.Chdir(cwd), "Changing to working directory should succeed")
		t.Cleanup(func() {
			require.NoError(t, os.Chdir(prev), "Working directory should be restored")
		})

		err = op.CreateAgent(operator.CreateAgentParams{Name: "operator-c"})
		require.NoError(t, err, "CreateAgent should auto-init memory in the current workspace")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{})
		require.NoError(t, err, "ListCodeAgents should default to resolved workspace")
		require.NotNil(t, result.AgentInfo, "Stored agent should be returned")
		assert.Equal(t, sandbox.WorkspaceDir(cwd), result.AgentInfo.WorkspaceDir, "Workspace should default to the current directory when no memory root exists")
		assert.FileExists(t, filepath.Join(cwd, "memory", "memory.yaml"), "Workspace memory root should be created automatically")
		assert.FileExists(t, filepath.Join(cwd, "agent_operator-c.md"), "Workspace agent markdown should be created")
	})

	t.Run("ResolvesWorkspaceFromNearestAncestorMemory", func(t *testing.T) {
		op := newTestOperator(t, true)
		workspace := t.TempDir()
		require.NoError(t, op.TeamInit(operator.TeamInitParams{Workspace: sandbox.WorkspaceDir(workspace)}), "TeamInit should prepare the ancestor workspace")

		nested := filepath.Join(workspace, "nested", "child")
		require.NoError(t, os.MkdirAll(nested, 0o755), "Nested working directory creation should succeed")

		prev, err := os.Getwd()
		require.NoError(t, err, "Current working directory lookup should succeed")
		require.NoError(t, os.Chdir(nested), "Changing to nested working directory should succeed")
		t.Cleanup(func() {
			require.NoError(t, os.Chdir(prev), "Working directory should be restored")
		})

		err = op.CreateAgent(operator.CreateAgentParams{Name: "operator-d"})
		require.NoError(t, err, "CreateAgent should resolve to the nearest ancestor with a memory root")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{})
		require.NoError(t, err, "ListCodeAgents should default to the resolved ancestor workspace")
		require.NotNil(t, result.AgentInfo, "Stored agent should be returned")
		assert.Equal(t, sandbox.WorkspaceDir(workspace), result.AgentInfo.WorkspaceDir, "Workspace should resolve to the ancestor containing memory")
		assert.FileExists(t, filepath.Join(workspace, "agent_operator-d.md"), "Workspace agent markdown should be created in the ancestor workspace")
	})

	t.Run("AllowsGeneratedNamesWhenFlagEnabled", func(t *testing.T) {
		op := newTestOperator(t, true)
		workspace := sandbox.WorkspaceDir(t.TempDir())

		err := op.CreateAgent(operator.CreateAgentParams{Workspace: workspace, AllowGeneratedName: true})
		require.NoError(t, err, "CreateAgent should allow generated names when the flag is enabled")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{Workspace: workspace})
		require.NoError(t, err, "ListCodeAgents should return the generated-name agent")
		require.NotNil(t, result.AgentInfo, "Generated-name agent should be stored")
		assert.Contains(t, result.AgentInfo.Name, "agent-", "Generated names should use the agent- prefix")
	})

	t.Run("InteractiveCreateBootstrapsAndResumesSession", func(t *testing.T) {
		op := newTestOperator(t, true)
		runtime := &stubCodeAgent{}
		op.newCodeAgent = func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			assert.Equal(t, codeagent.Provider(operator.DefaultProvider), provider, "CreateAgent should use the default provider")
			assert.Equal(t, operator.DefaultModel, model, "CreateAgent should use the default model")
			assert.NotEmpty(t, workDir, "CreateAgent should pass a workspace to the connector")
			return runtime, nil
		}

		workspace := sandbox.WorkspaceDir(t.TempDir())
		err := op.CreateAgent(operator.CreateAgentParams{Workspace: workspace, Name: "operator-interactive", Interactive: true})
		require.NoError(t, err, "Interactive CreateAgent should bootstrap and resume the initial session")

		require.Len(t, runtime.createCalls, 1, "CreateAgent should create exactly one connector session")
		assert.Equal(t, "operator-interactive", runtime.createCalls[0].Name, "Connector session should inherit the agent name")
		assert.Equal(t, string(workspace), runtime.createCalls[0].WorkDir, "Connector session should target the workspace")
		require.Len(t, runtime.resumeCalls, 1, "Interactive CreateAgent should resume the created session")
		assert.Equal(t, runtime.createResultID, runtime.resumeCalls[0].ID, "Resume should target the created session ID")
	})

	t.Run("NonInteractiveCreateSkipsResume", func(t *testing.T) {
		op := newTestOperator(t, true)
		runtime := &stubCodeAgent{}
		op.newCodeAgent = func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			return runtime, nil
		}

		err := op.CreateAgent(operator.CreateAgentParams{
			Workspace:   sandbox.WorkspaceDir(t.TempDir()),
			Name:        "operator-batch",
			Interactive: false,
		})
		require.NoError(t, err, "Non-interactive CreateAgent should still create the initial session")

		require.Len(t, runtime.createCalls, 1, "CreateAgent should still bootstrap a connector session")
		assert.Empty(t, runtime.resumeCalls, "Non-interactive CreateAgent should not resume the session")
	})

	t.Run("CreateAgentReturnsSessionBootstrapErrors", func(t *testing.T) {
		op := newTestOperator(t, true)
		op.newCodeAgent = func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			return nil, errors.New("connector unavailable")
		}

		err := op.CreateAgent(operator.CreateAgentParams{
			Workspace:   sandbox.WorkspaceDir(t.TempDir()),
			Name:        "operator-fail",
			Interactive: true,
		})
		require.EqualError(t, err, "operator: init code agent runtime: connector unavailable", "CreateAgent should surface connector bootstrap failures")
	})
}

func TestTeamInit(t *testing.T) {
	t.Run("MemoryEnabledInitialisesRootTemplate", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is required for TeamInit")
		}

		op := newTestOperator(t, true)
		workspace := t.TempDir()

		err := op.TeamInit(operator.TeamInitParams{Workspace: sandbox.WorkspaceDir(workspace)})
		require.NoError(t, err, "TeamInit should succeed when memory is enabled")

		assert.FileExists(t, filepath.Join(workspace, "memory", "memory.yaml"), "Workspace memory.yaml should be created")
		assert.FileExists(t, filepath.Join(workspace, "memory", "semantics.yaml"), "Workspace semantics.yaml should be created")
		assert.FileExists(t, filepath.Join(workspace, "memory", "metadata.yaml"), "memory metadata should be created")
		assert.DirExists(t, filepath.Join(workspace, "memory", ".git"), "memory directory should be initialised as a git repo")
		assert.FileExists(t, filepath.Join(workspace, "agent_memory.md"), "Workspace should include the memory entry markdown")
		assert.DirExists(t, filepath.Join(workspace, "memory", "guide"), "TeamInit should create the default guide agent")
		_, guideMemoryErr := os.Stat(filepath.Join(workspace, "memory", "guide", "entry", "data", "memory.yaml"))
		assert.ErrorIs(t, guideMemoryErr, os.ErrNotExist, "Guide agent should not receive per-agent memory data files")
		assert.FileExists(t, filepath.Join(workspace, "agent_guide.md"), "Guide agent workspace markdown should be created")

		result, err := op.ListCodeAgents(operator.GetCodeAgentsParams{Workspace: sandbox.WorkspaceDir(workspace)})
		require.NoError(t, err, "ListCodeAgents should resolve agents for the initialised workspace")
		require.NotNil(t, result.AgentInfo, "Guide agent should be stored")
		assert.Equal(t, "guide", result.AgentInfo.Name, "TeamInit should create the guide agent by default")
	})

	t.Run("MemoryDisabledReturnsError", func(t *testing.T) {
		op := newTestOperator(t, false)
		err := op.TeamInit(operator.TeamInitParams{Workspace: sandbox.WorkspaceDir(t.TempDir())})
		require.ErrorIs(t, err, operator.ErrMemoryDisabled, "TeamInit should reject calls when memory is disabled")
	})

	t.Run("DefaultsWorkspaceToCwd", func(t *testing.T) {
		if _, err := exec.LookPath("git"); err != nil {
			t.Skip("git is required for TeamInit")
		}

		op := newTestOperator(t, true)
		workspace := t.TempDir()

		prev, err := os.Getwd()
		require.NoError(t, err, "Current working directory lookup should succeed")
		require.NoError(t, os.Chdir(workspace), "Changing to workspace should succeed")
		t.Cleanup(func() {
			require.NoError(t, os.Chdir(prev), "Working directory should be restored")
		})

		err = op.TeamInit(operator.TeamInitParams{})
		require.NoError(t, err, "TeamInit should default workspace to cwd")
		assert.FileExists(t, filepath.Join(workspace, "memory", "memory.yaml"), "Memory root should be created in cwd")
	})
}

func TestValidation(t *testing.T) {
	t.Run("ParamValidation", func(t *testing.T) {
		assert.NoError(t, (operator.GetCodeAgentsParams{}).Validate(), "GetCodeAgentsParams should allow empty workspace")
		assert.EqualError(t, (operator.CreateAgentParams{}).Validate(), "operator: agent name is required unless generated names are enabled", "CreateAgentParams should require a name unless generated names are enabled")
		assert.NoError(t, (operator.CreateAgentParams{AllowGeneratedName: true}).Validate(), "Generated-name flag should bypass explicit-name validation")
		assert.EqualError(t, (operator.DeleteAgentParams{}).Validate(), "operator: agent id is required", "DeleteAgentParams should require agent id")
		assert.EqualError(t, (operator.GetWorkSpaceParams{}).Validate(), "operator: workspace id is required", "GetWorkSpaceParams should require workspace id")
		assert.NoError(t, (operator.TeamInitParams{}).Validate(), "TeamInitParams should allow empty workspace")
		assert.EqualError(t, (operator.UpgradeAgentParams{}).Validate(), "operator: agent id is required", "UpgradeAgentParams should require agent id")
	})

	t.Run("OperatorMethodsRejectInvalidParams", func(t *testing.T) {
		op := newTestOperator(t, true)

		err := op.CreateAgent(operator.CreateAgentParams{})
		require.EqualError(t, err, "operator: agent name is required unless generated names are enabled", "CreateAgent should reject missing names by default")

		_, err = op.GetWorkspace(operator.GetWorkSpaceParams{})
		require.EqualError(t, err, "operator: workspace id is required", "GetWorkspace should reject empty workspace id")

		err = op.DeleteAgent(operator.DeleteAgentParams{})
		require.EqualError(t, err, "operator: agent id is required", "DeleteAgent should reject empty id")

		err = op.UpgradeAgent(operator.UpgradeAgentParams{})
		require.EqualError(t, err, "operator: agent id is required", "UpgradeAgent should reject empty id")
	})
}

func TestResumeAgent(t *testing.T) {
	t.Run("ResumesNamedAgentFromWorkspaceIndex", func(t *testing.T) {
		resumed := &stubCodeAgent{}
		op := newTestOperator(t, true)
		op.newCodeAgent = func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			return resumed, nil
		}

		workspace := sandbox.WorkspaceDir(t.TempDir())
		err := op.CreateAgent(operator.CreateAgentParams{Workspace: workspace, Name: "operator-resume"})
		require.NoError(t, err, "CreateAgent should succeed before resume")

		err = op.ResumeAgent(operator.ResumeAgentParams{Workspace: workspace, Name: "operator-resume"})
		require.NoError(t, err, "ResumeAgent should succeed for an indexed agent")
		require.Len(t, resumed.resumeCalls, 1, "Resume should be invoked exactly once")
		assert.NotEmpty(t, resumed.resumeCalls[0].ID, "Resume should be called with a session identifier")
	})

	t.Run("RejectsMissingName", func(t *testing.T) {
		op := newTestOperator(t, true)
		err := op.ResumeAgent(operator.ResumeAgentParams{})
		require.EqualError(t, err, "operator: agent name is required", "ResumeAgent should require an agent name")
	})
}

func newTestOperator(t *testing.T, memoryEnabled bool) *DefaultOperator {
	t.Helper()

	store := operatorstore.NewWithDB(newTestDB(t))
	op := &DefaultOperator{
		config: config.OmniConfig{
			Features: &config.Features{Memory: memoryEnabled},
		},
		store: store,
		newCodeAgent: func(provider codeagent.Provider, workDir, model string) (codeagent.CodeAgent, error) {
			return &stubCodeAgent{}, nil
		},
	}
	if memoryEnabled {
		op.agentMemory = operator.NewDefaultAgentMemory()
	}
	return op
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "operator-test.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err, "Opening temp sqlite database should succeed")
	t.Cleanup(func() {
		require.NoError(t, db.Close(), "Closing temp sqlite database should succeed")
	})

	_, err = db.Exec(testAgentSchema)
	require.NoError(t, err, "Creating agent tables should succeed")

	_, err = db.Exec(testWorkspaceSchema)
	require.NoError(t, err, "Creating workspaces table should succeed")

	return db
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()

	row := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, name)
	var count int
	require.NoError(t, row.Scan(&count), "sqlite_master lookup should succeed")
	return count == 1
}

type stubCodeAgent struct {
	createResultID string
	createCalls    []codeagent.CreateSessionParams
	resumeCalls    []codeagent.ResumeSessionParams
}

func (s *stubCodeAgent) Create(p codeagent.CreateSessionParams) (*codeagent.CreateSessionResult, error) {
	s.createCalls = append(s.createCalls, p)
	if s.createResultID == "" {
		s.createResultID = p.ID
		if s.createResultID == "" {
			s.createResultID = "stub-session"
		}
	}
	return &codeagent.CreateSessionResult{ID: s.createResultID, Name: p.Name}, nil
}

func (s *stubCodeAgent) Exec(codeagent.ExecuteParams) (*codeagent.ExecuteResult, error) {
	return &codeagent.ExecuteResult{}, nil
}

func (s *stubCodeAgent) Stream(codeagent.StreamParams) (*codeagent.StreamResult, error) {
	ch := make(chan codeagent.StreamEvent)
	close(ch)
	return &codeagent.StreamResult{Events: ch}, nil
}

func (s *stubCodeAgent) Resume(p codeagent.ResumeSessionParams) (*codeagent.ResumeSessionResult, error) {
	s.resumeCalls = append(s.resumeCalls, p)
	return &codeagent.ResumeSessionResult{SessionID: p.ID, ProcessID: "123"}, nil
}

func (s *stubCodeAgent) List(codeagent.ListSessionsParams) (*codeagent.ListSessionsResult, error) {
	return &codeagent.ListSessionsResult{}, nil
}

func (s *stubCodeAgent) Delete(codeagent.DeleteSessionParams) (*codeagent.DeleteSessionResult, error) {
	return &codeagent.DeleteSessionResult{}, nil
}

func (s *stubCodeAgent) GetSessionConfig(codeagent.GetSessionConfigParams) (*codeagent.GetSessionConfigResult, error) {
	return &codeagent.GetSessionConfigResult{}, nil
}

func (s *stubCodeAgent) GetSessionSandbox(codeagent.GetSessionSandboxParams) (*codeagent.GetSessionSandboxResult, error) {
	return &codeagent.GetSessionSandboxResult{}, nil
}

func (s *stubCodeAgent) UpdateSessionSandbox(codeagent.UpdateSessionSandboxParams) (*codeagent.UpdateSessionSandboxResult, error) {
	return &codeagent.UpdateSessionSandboxResult{}, nil
}

func (s *stubCodeAgent) Capabilities() (*codeagent.Capabilities, error) {
	return &codeagent.Capabilities{}, nil
}

func (s *stubCodeAgent) Info() *codeagent.CodeAgentInfo {
	return &codeagent.CodeAgentInfo{}
}

func (s *stubCodeAgent) GetUserIdentity() codeagent.UserIdentify {
	return codeagent.UserIdentify{}
}

func (s *stubCodeAgent) Stop() {}

func (s *stubCodeAgent) Defaults() (*codeagent.Config, error) {
	return &codeagent.Config{}, nil
}

func (s *stubCodeAgent) UpdateDefaults(*codeagent.Config) error {
	return nil
}

func (s *stubCodeAgent) GetUserSettings() (*codeagent.Settings, error) {
	return &codeagent.Settings{}, nil
}

func (s *stubCodeAgent) GetWorkspaceSettings(sandbox.WorkspaceDir) (*codeagent.Settings, error) {
	return &codeagent.Settings{}, nil
}

func (s *stubCodeAgent) SaveDefaultSettings(*codeagent.Settings) error {
	return nil
}

func (s *stubCodeAgent) WatchDefaultSettings(func(*codeagent.Settings)) error {
	return nil
}

func (s *stubCodeAgent) SupportedHooks() (*hooks.Capabilities, error) {
	return &hooks.Capabilities{}, nil
}

func (s *stubCodeAgent) Register(hooks.RegisterHookParams) error {
	return nil
}

func (s *stubCodeAgent) GetRegisteredHooks() []*hooks.HookData {
	return nil
}

func (s *stubCodeAgent) DeleteHook(hooks.DeleteHookParams) (bool, error) {
	return false, nil
}

func (s *stubCodeAgent) PreToolUseParams(any) (*hooks.PreToolUseParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostToolUseParams(any) (*hooks.PostToolUseParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostToolUseFailureParams(any) (*hooks.PostToolUseFailureParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PreSessionStartParams(any) (*hooks.PreSessionStartParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostSessionStartParams(any) (*hooks.PostSessionStartParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PrePromptInputParams(any) (*hooks.PrePromptInputParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostPromptInputParams(any) (*hooks.PostPromptInputParams, error) {
	return nil, nil
}

func (s *stubCodeAgent) PreToolUseResult(any) (*hooks.PreToolUseResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostToolUseResult(any) (*hooks.PostToolUseResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostToolUseFailureResult(any) (*hooks.PostToolUseFailureResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PreSessionStartResult(any) (*hooks.PreSessionStartResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostSessionStartResult(any) (*hooks.PostSessionStartResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PrePromptInputResult(any) (*hooks.PrePromptInputResult, error) {
	return nil, nil
}

func (s *stubCodeAgent) PostPromptInputResult(any) (*hooks.PostPromptInputResult, error) {
	return nil, nil
}
