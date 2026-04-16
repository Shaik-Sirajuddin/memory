package operator

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/omniagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent/database"
	sandbox "github.com/Shaik-Sirajuddin/memory/sandbox/provider"
)

const workspaceSchema = `
CREATE TABLE IF NOT EXISTS workspaces (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    workspace_dir TEXT NOT NULL UNIQUE
);`

// OperatorStore handles persistence for the operator layer.
// It owns the workspaces table and reads the agents table (owned by omniagent).
type OperatorStore interface {
	CreateWorkspace(ws *TeamInfo) error
	GetWorkspace(id string) (*TeamInfo, error)
	WorkspaceByDir(dir sandbox.WorkspaceDir) (*TeamInfo, error)
	ListWorkspaces() ([]*TeamInfo, error)
	DeleteWorkspace(id string) error

	// Agent operations — read/write omniagent's agents table via the shared DB.
	ListAgentsByDir(dir sandbox.WorkspaceDir) ([]*omniagent.AgentInfo, error)
	GetAgent(id string) (*omniagent.AgentInfo, error)
	CreateAgent(agent *omniagent.AgentInfo) error
	DeleteAgent(id string) error
}

type sqlStore struct {
	db *sql.DB
}

var (
	storeOnce sync.Once
	opStore   *sqlStore
	storeErr  error
)

// GetOperatorStore returns the singleton OperatorStore, initializing it on first call.
// It reuses the omniagent DB singleton and migrates the workspaces table if absent.
func GetOperatorStore() (OperatorStore, error) {
	storeOnce.Do(func() {
		logger.Debug("GetOperatorStore: initialising store singleton")
		db, err := database.GetDB()
		if err != nil {
			logger.Error("GetOperatorStore: database init failed", "err", err)
			storeErr = err
			return
		}
		if _, err = db.Exec(workspaceSchema); err != nil {
			logger.Error("GetOperatorStore: workspace migration failed", "err", err)
			storeErr = fmt.Errorf("operator: migrate workspaces table: %w", err)
			return
		}
		opStore = &sqlStore{db: db}
		logger.Info("GetOperatorStore: store ready")
	})
	return opStore, storeErr
}

// --- Workspace CRUD ---

func (s *sqlStore) CreateWorkspace(ws *TeamInfo) error {
	logger.Debug("CreateWorkspace: insert", "workspaceID", ws.ID, "workspaceDir", ws.WorkspaceDir)
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, name, workspace_dir) VALUES (?, ?, ?)`,
		ws.ID, ws.Name, ws.WorkspaceDir,
	)
	if err != nil {
		logger.Error("CreateWorkspace: insert failed", "workspaceID", ws.ID, "workspaceDir", ws.WorkspaceDir, "err", err)
		return err
	}
	logger.Info("CreateWorkspace: inserted", "workspaceID", ws.ID, "workspaceDir", ws.WorkspaceDir)
	return err
}

func (s *sqlStore) GetWorkspace(id string) (*TeamInfo, error) {
	logger.Debug("GetWorkspace(store): query", "workspaceID", id)
	row := s.db.QueryRow(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w WHERE w.id = ?`, id,
	)
	ws, err := scanWorkspace(row)
	if err != nil {
		logger.Error("GetWorkspace(store): query failed", "workspaceID", id, "err", err)
		return nil, err
	}
	logger.Info("GetWorkspace(store): loaded", "workspaceID", ws.ID, "agentCount", ws.Agents)
	return ws, nil
}

func (s *sqlStore) WorkspaceByDir(dir sandbox.WorkspaceDir) (*TeamInfo, error) {
	logger.Debug("WorkspaceByDir: query", "workspaceDir", dir)
	row := s.db.QueryRow(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w WHERE w.workspace_dir = ?`, string(dir),
	)
	ws, err := scanWorkspace(row)
	if err != nil {
		logger.Debug("WorkspaceByDir: query failed", "workspaceDir", dir, "err", err)
		return nil, err
	}
	logger.Info("WorkspaceByDir: loaded", "workspaceID", ws.ID, "workspaceDir", ws.WorkspaceDir)
	return ws, nil
}

func (s *sqlStore) ListWorkspaces() ([]*TeamInfo, error) {
	logger.Debug("ListWorkspaces(store): query")
	rows, err := s.db.Query(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w`,
	)
	if err != nil {
		logger.Error("ListWorkspaces(store): query failed", "err", err)
		return nil, err
	}
	defer rows.Close()

	var teams []*TeamInfo
	for rows.Next() {
		var t TeamInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.WorkspaceDir, &t.Agents); err != nil {
			logger.Error("ListWorkspaces(store): scan failed", "err", err)
			return nil, err
		}
		teams = append(teams, &t)
	}
	if err := rows.Err(); err != nil {
		logger.Error("ListWorkspaces(store): rows iteration failed", "err", err)
		return nil, err
	}
	logger.Info("ListWorkspaces(store): completed", "count", len(teams))
	return teams, nil
}

func (s *sqlStore) DeleteWorkspace(id string) error {
	logger.Debug("DeleteWorkspace: delete", "workspaceID", id)
	res, err := s.db.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		logger.Error("DeleteWorkspace: delete failed", "workspaceID", id, "err", err)
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		logger.Error("DeleteWorkspace: rows affected failed", "workspaceID", id, "err", err)
		return err
	}
	if n == 0 {
		logger.Error("DeleteWorkspace: workspace not found", "workspaceID", id)
		return fmt.Errorf("operator: workspace %q not found", id)
	}
	logger.Info("DeleteWorkspace: completed", "workspaceID", id)
	return nil
}

// --- Agent operations (omniagent's agents table) ---

func (s *sqlStore) ListAgentsByDir(dir sandbox.WorkspaceDir) ([]*omniagent.AgentInfo, error) {
	logger.Debug("ListAgentsByDir: query", "workspaceDir", dir)
	rows, err := s.db.Query(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE workspace_dir = ?`,
		string(dir),
	)
	if err != nil {
		logger.Error("ListAgentsByDir: query failed", "workspaceDir", dir, "err", err)
		return nil, err
	}
	defer rows.Close()

	var agents []*omniagent.AgentInfo
	for rows.Next() {
		info, err := scanAgent(rows)
		if err != nil {
			logger.Error("ListAgentsByDir: scan failed", "workspaceDir", dir, "err", err)
			return nil, err
		}
		agents = append(agents, info)
	}
	if err := rows.Err(); err != nil {
		logger.Error("ListAgentsByDir: rows iteration failed", "workspaceDir", dir, "err", err)
		return nil, err
	}
	logger.Info("ListAgentsByDir: completed", "workspaceDir", dir, "count", len(agents))
	return agents, nil
}

func (s *sqlStore) GetAgent(id string) (*omniagent.AgentInfo, error) {
	logger.Debug("GetAgent(store): query", "agentID", id)
	row := s.db.QueryRow(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE id = ?`, id,
	)
	var info omniagent.AgentInfo
	var wsDir string
	if err := row.Scan(&info.ID, &info.Name, &wsDir, &info.MemoryDir); err != nil {
		logger.Error("GetAgent(store): query failed", "agentID", id, "err", err)
		return nil, err
	}
	info.WorkspaceDir = sandbox.WorkspaceDir(wsDir)
	logger.Info("GetAgent(store): loaded", "agentID", id, "workspaceDir", wsDir)
	return &info, nil
}

func (s *sqlStore) CreateAgent(agent *omniagent.AgentInfo) error {
	logger.Debug("CreateAgent(store): insert", "agentID", agent.ID, "workspaceDir", agent.WorkspaceDir)
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, workspace_dir, memory_dir) VALUES (?, ?, ?, ?)`,
		agent.ID, agent.Name, string(agent.WorkspaceDir), agent.MemoryDir,
	)
	if err != nil {
		logger.Error("CreateAgent(store): insert failed", "agentID", agent.ID, "workspaceDir", agent.WorkspaceDir, "err", err)
		return err
	}
	logger.Info("CreateAgent(store): inserted", "agentID", agent.ID, "workspaceDir", agent.WorkspaceDir)
	return err
}

func (s *sqlStore) DeleteAgent(id string) error {
	logger.Debug("DeleteAgent(store): delete", "agentID", id)
	res, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		logger.Error("DeleteAgent(store): delete failed", "agentID", id, "err", err)
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		logger.Error("DeleteAgent(store): rows affected failed", "agentID", id, "err", err)
		return err
	}
	if n == 0 {
		logger.Error("DeleteAgent(store): agent not found", "agentID", id)
		return fmt.Errorf("operator: agent %q not found", id)
	}
	logger.Info("DeleteAgent(store): completed", "agentID", id)
	return nil
}

// --- helpers ---

func scanWorkspace(row *sql.Row) (*TeamInfo, error) {
	var t TeamInfo
	if err := row.Scan(&t.ID, &t.Name, &t.WorkspaceDir, &t.Agents); err != nil {
		return nil, err
	}
	return &t, nil
}

func scanAgent(rows *sql.Rows) (*omniagent.AgentInfo, error) {
	var info omniagent.AgentInfo
	var wsDir string
	if err := rows.Scan(&info.ID, &info.Name, &wsDir, &info.MemoryDir); err != nil {
		return nil, err
	}
	info.WorkspaceDir = sandbox.WorkspaceDir(wsDir)
	return &info, nil
}
