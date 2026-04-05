package store

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent"
	"github.com/Shaik-Sirajuddin/memory/omniagent/database"
)

type sqlCodeSessionStore struct {
	db *sql.DB
}

var (
	sessionStoreOnce sync.Once
	sessionStore     *sqlCodeSessionStore
	sessionStoreErr  error
)

// GetCodeSessionStore returns the singleton CodeSessionStore.
func GetCodeSessionStore() (CodeSessionStore, error) {
	sessionStoreOnce.Do(func() {
		db, err := database.GetDB()
		if err != nil {
			sessionStoreErr = err
			return
		}
		sessionStore = &sqlCodeSessionStore{db: db}
	})
	return sessionStore, sessionStoreErr
}

// GetSession returns the active session for the given agent.
func (s *sqlCodeSessionStore) GetSession(agentID string) (*omniagent.CodeSession, error) {
	row := s.db.QueryRow(
		`SELECT id, model_provider, model_name, idx, is_active, prompts, last_sync_prompt
		 FROM code_sessions WHERE agent_id = ? AND is_active = 1 LIMIT 1`,
		agentID,
	)
	return scanSession(row)
}

// CreateSession persists a new session for the given agent.
func (s *sqlCodeSessionStore) CreateSession(agentID string, session *omniagent.CodeSession) error {
	provider, name := modelFields(session.Model)
	_, err := s.db.Exec(
		`INSERT INTO code_sessions (id, agent_id, model_provider, model_name, idx, is_active, prompts, last_sync_prompt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.Id, agentID, provider, name,
		session.Idx, boolToInt(session.IsActive), session.Prompts, session.LastSyncPrompt,
	)
	return err
}

// UpdateSession updates an existing session for the given agent.
func (s *sqlCodeSessionStore) UpdateSession(agentID string, session *omniagent.CodeSession) error {
	provider, name := modelFields(session.Model)
	res, err := s.db.Exec(
		`UPDATE code_sessions
		 SET model_provider = ?, model_name = ?, idx = ?, is_active = ?, prompts = ?, last_sync_prompt = ?
		 WHERE id = ? AND agent_id = ?`,
		provider, name, session.Idx, boolToInt(session.IsActive), session.Prompts, session.LastSyncPrompt,
		session.Id, agentID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("session %q not found for agent %q", session.Id, agentID)
	}
	return nil
}

// ListSessions returns sessions for the given agent, filtered by non-zero fields of filter.
func (s *sqlCodeSessionStore) ListSessions(agentID string, filter *omniagent.CodeSession) ([]*omniagent.CodeSession, error) {
	query := `SELECT id, model_provider, model_name, idx, is_active, prompts, last_sync_prompt
	          FROM code_sessions WHERE agent_id = ?`
	args := []any{agentID}

	if filter != nil {
		if filter.Id != "" {
			query += " AND id = ?"
			args = append(args, filter.Id)
		}
		if filter.IsActive {
			query += " AND is_active = 1"
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*omniagent.CodeSession
	for rows.Next() {
		session, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// --- helpers ---

func scanSession(row *sql.Row) (*omniagent.CodeSession, error) {
	var (
		id, provider, modelName string
		idx, prompts, lastSync  int
		isActive                int
	)
	if err := row.Scan(&id, &provider, &modelName, &idx, &isActive, &prompts, &lastSync); err != nil {
		return nil, err
	}
	return &omniagent.CodeSession{
		Id:             id,
		Model:          buildModel(provider, modelName),
		Idx:            idx,
		IsActive:       isActive == 1,
		Prompts:        prompts,
		LastSyncPrompt: lastSync,
	}, nil
}

func scanSessionRow(rows *sql.Rows) (*omniagent.CodeSession, error) {
	var (
		id, provider, modelName string
		idx, prompts, lastSync  int
		isActive                int
	)
	if err := rows.Scan(&id, &provider, &modelName, &idx, &isActive, &prompts, &lastSync); err != nil {
		return nil, err
	}
	return &omniagent.CodeSession{
		Id:             id,
		Model:          buildModel(provider, modelName),
		Idx:            idx,
		IsActive:       isActive == 1,
		Prompts:        prompts,
		LastSyncPrompt: lastSync,
	}, nil
}

func modelFields(m *codeagent.Model) (provider, name string) {
	if m == nil {
		return "", ""
	}
	return string(m.Provider), m.Model
}

func buildModel(provider, name string) *codeagent.Model {
	if provider == "" && name == "" {
		return nil
	}
	return &codeagent.Model{Provider: codeagent.Provider(provider), Model: name}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
