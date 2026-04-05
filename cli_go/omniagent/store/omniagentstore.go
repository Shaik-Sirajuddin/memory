package store

import (
	"database/sql"
	"encoding/json"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent/database"
)

type sqlOmniAgentStore struct {
	db       *sql.DB
	sessions CodeSessionStore
}

var (
	omniStoreOnce sync.Once
	omniStore     *sqlOmniAgentStore
	omniStoreErr  error
)

// GetOmniAgentStore returns the singleton OmniAgentStore.
func GetOmniAgentStore() (OmniAgentStore, error) {
	omniStoreOnce.Do(func() {
		db, err := database.GetDB()
		if err != nil {
			omniStoreErr = err
			return
		}
		sessions, err := GetCodeSessionStore()
		if err != nil {
			omniStoreErr = err
			return
		}
		omniStore = &sqlOmniAgentStore{db: db, sessions: sessions}
	})
	return omniStore, omniStoreErr
}

// Save upserts the agent's scalar fields (info + settings). Array fields are not persisted here.
func (s *sqlOmniAgentStore) Save(agent *omniagent.Data) error {
	if err := s.upsertInfo(agent.Info); err != nil {
		return err
	}
	if agent.Settings != nil {
		return s.UpdateSettings(agent.Info.ID, agent.Settings)
	}
	return nil
}

// Create inserts a new agent record with info and default settings.
func (s *sqlOmniAgentStore) Create(agent *omniagent.Data) error {
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, workspace_dir, memory_dir) VALUES (?, ?, ?, ?)`,
		agent.Info.ID, agent.Info.Name, string(agent.Info.WorkspaceDir), agent.Info.MemoryDir,
	)
	if err != nil {
		return err
	}
	settings := agent.Settings
	if settings == nil {
		settings = &omniagent.Settings{}
	}
	return s.UpdateSettings(agent.Info.ID, settings)
}

// GetAgent returns an agent's data. Sessions array is omitted.
func (s *sqlOmniAgentStore) GetAgent(ID string) (*omniagent.Data, error) {
	row := s.db.QueryRow(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE id = ?`, ID,
	)
	info, err := scanAgentInfo(row)
	if err != nil {
		return nil, err
	}
	settings, err := s.GetSettings(ID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return &omniagent.Data{Info: info, Settings: settings}, nil
}

// GetActiveSession delegates to CodeSessionStore.
func (s *sqlOmniAgentStore) GetActiveSession(ID string) (*omniagent.CodeSession, error) {
	return s.sessions.GetSession(ID)
}

// UpdateActiveSession delegates to CodeSessionStore.
func (s *sqlOmniAgentStore) UpdateActiveSession(ID string, session *omniagent.CodeSession) error {
	return s.sessions.UpdateSession(ID, session)
}

// CreateSession delegates to CodeSessionStore.
func (s *sqlOmniAgentStore) CreateSession(ID string, session *omniagent.CodeSession) error {
	return s.sessions.CreateSession(ID, session)
}

// GetSettings returns the settings for the given agent.
func (s *sqlOmniAgentStore) GetSettings(ID string) (*omniagent.Settings, error) {
	row := s.db.QueryRow(
		`SELECT sandbox, default_model_provider, default_model_name
		 FROM agent_settings WHERE agent_id = ?`, ID,
	)
	var sandboxJSON, provider, modelName string
	if err := row.Scan(&sandboxJSON, &provider, &modelName); err != nil {
		return nil, err
	}
	settings := &omniagent.Settings{
		DefaultModel: buildModel(provider, modelName),
	}
	if sandboxJSON != "" && sandboxJSON != "{}" {
		var sb sandbox.Sandbox
		if err := json.Unmarshal([]byte(sandboxJSON), &sb); err != nil {
			return nil, err
		}
		settings.Sandbox = &sb
	}
	return settings, nil
}

// UpdateSettings upserts settings for the given agent.
func (s *sqlOmniAgentStore) UpdateSettings(ID string, settings *omniagent.Settings) error {
	sandboxJSON, err := marshalSandbox(settings.Sandbox)
	if err != nil {
		return err
	}
	provider, modelName := modelFields(settings.DefaultModel)
	_, err = s.db.Exec(
		`INSERT INTO agent_settings (agent_id, sandbox, default_model_provider, default_model_name)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(agent_id) DO UPDATE SET
		   sandbox = excluded.sandbox,
		   default_model_provider = excluded.default_model_provider,
		   default_model_name = excluded.default_model_name`,
		ID, sandboxJSON, provider, modelName,
	)
	return err
}

// ListAgents queries agents filtered by workspace.
func (s *sqlOmniAgentStore) ListAgents(params ListAgentParams) {
	_, _ = s.db.Query(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE workspace_dir = ?`,
		string(params.Workspace),
	)
}

// DeleteAgent is defined by the interface; agent ID is not provided at this level.
func (s *sqlOmniAgentStore) DeleteAgent() {}

// --- helpers ---

func (s *sqlOmniAgentStore) upsertInfo(info *omniagent.AgentInfo) error {
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, workspace_dir, memory_dir) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name,
		   workspace_dir = excluded.workspace_dir,
		   memory_dir = excluded.memory_dir`,
		info.ID, info.Name, string(info.WorkspaceDir), info.MemoryDir,
	)
	return err
}

func scanAgentInfo(row *sql.Row) (*omniagent.AgentInfo, error) {
	var id, name, workspaceDir, memoryDir string
	if err := row.Scan(&id, &name, &workspaceDir, &memoryDir); err != nil {
		return nil, err
	}
	return &omniagent.AgentInfo{
		ID:           id,
		Name:         name,
		WorkspaceDir: sandbox.WorkspaceDir(workspaceDir),
		MemoryDir:    memoryDir,
	}, nil
}

func marshalSandbox(sb *sandbox.Sandbox) (string, error) {
	if sb == nil {
		return "{}", nil
	}
	b, err := json.Marshal(sb)
	return string(b), err
}

