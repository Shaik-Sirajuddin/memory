package operator

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/connector/sandbox"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent/database"
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

type sqlOperatorStore struct {
	db *sql.DB
}

var (
	storeOnce sync.Once
	opStore   *sqlOperatorStore
	storeErr  error
)

// GetOperatorStore returns the singleton OperatorStore, initializing it on first call.
// It reuses the omniagent DB singleton and migrates the workspaces table if absent.
func GetOperatorStore() (OperatorStore, error) {
	storeOnce.Do(func() {
		db, err := database.GetDB()
		if err != nil {
			storeErr = err
			return
		}
		if _, err = db.Exec(workspaceSchema); err != nil {
			storeErr = fmt.Errorf("operator: migrate workspaces table: %w", err)
			return
		}
		opStore = &sqlOperatorStore{db: db}
	})
	return opStore, storeErr
}

// --- Workspace CRUD ---

func (s *sqlOperatorStore) CreateWorkspace(ws *TeamInfo) error {
	_, err := s.db.Exec(
		`INSERT INTO workspaces (id, name, workspace_dir) VALUES (?, ?, ?)`,
		ws.ID, ws.Name, ws.WorkspaceDir,
	)
	return err
}

func (s *sqlOperatorStore) GetWorkspace(id string) (*TeamInfo, error) {
	row := s.db.QueryRow(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w WHERE w.id = ?`, id,
	)
	return scanWorkspace(row)
}

func (s *sqlOperatorStore) WorkspaceByDir(dir sandbox.WorkspaceDir) (*TeamInfo, error) {
	row := s.db.QueryRow(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w WHERE w.workspace_dir = ?`, string(dir),
	)
	return scanWorkspace(row)
}

func (s *sqlOperatorStore) ListWorkspaces() ([]*TeamInfo, error) {
	rows, err := s.db.Query(
		`SELECT w.id, w.name, w.workspace_dir,
		        (SELECT COUNT(*) FROM agents a WHERE a.workspace_dir = w.workspace_dir)
		 FROM workspaces w`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*TeamInfo
	for rows.Next() {
		var t TeamInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.WorkspaceDir, &t.Agents); err != nil {
			return nil, err
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}

func (s *sqlOperatorStore) DeleteWorkspace(id string) error {
	res, err := s.db.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("operator: workspace %q not found", id)
	}
	return nil
}

// --- Agent operations (omniagent's agents table) ---

func (s *sqlOperatorStore) ListAgentsByDir(dir sandbox.WorkspaceDir) ([]*omniagent.AgentInfo, error) {
	rows, err := s.db.Query(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE workspace_dir = ?`,
		string(dir),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*omniagent.AgentInfo
	for rows.Next() {
		info, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, info)
	}
	return agents, rows.Err()
}

func (s *sqlOperatorStore) GetAgent(id string) (*omniagent.AgentInfo, error) {
	row := s.db.QueryRow(
		`SELECT id, name, workspace_dir, memory_dir FROM agents WHERE id = ?`, id,
	)
	var info omniagent.AgentInfo
	var wsDir string
	if err := row.Scan(&info.ID, &info.Name, &wsDir, &info.MemoryDir); err != nil {
		return nil, err
	}
	info.WorkspaceDir = sandbox.WorkspaceDir(wsDir)
	return &info, nil
}

func (s *sqlOperatorStore) CreateAgent(agent *omniagent.AgentInfo) error {
	_, err := s.db.Exec(
		`INSERT INTO agents (id, name, workspace_dir, memory_dir) VALUES (?, ?, ?, ?)`,
		agent.ID, agent.Name, string(agent.WorkspaceDir), agent.MemoryDir,
	)
	return err
}

func (s *sqlOperatorStore) DeleteAgent(id string) error {
	res, err := s.db.Exec(`DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("operator: agent %q not found", id)
	}
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
