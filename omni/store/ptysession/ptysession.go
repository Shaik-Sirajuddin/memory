package ptysession

import (
	"database/sql"
	"time"
)

// PtySession tracks a live PTY process for an agent session.
type PtySession struct {
	AgentID   string
	SessionID string
	PID       int
	Status    string // "active" | "stopped" | "crashed"
	StartedAt time.Time
	StoppedAt *time.Time
}

// PtySessionStore persists live PTY process state separate from code sessions.
type PtySessionStore interface {
	GetPtySession(agentID, sessionID string) (*PtySession, error)
	UpsertPtySession(session *PtySession) error
	UpdatePtyStatus(agentID, sessionID, status string, stoppedAt *time.Time) error
	ListPtySessions(agentID string) ([]*PtySession, error)
}

type sqlPtySessionStore struct {
	db *sql.DB
}

// NewSqlPtySessionStore returns a PtySessionStore backed by the provided database.
func NewSqlPtySessionStore(db *sql.DB) PtySessionStore {
	return &sqlPtySessionStore{db: db}
}

func (s *sqlPtySessionStore) GetPtySession(agentID, sessionID string) (*PtySession, error) {
	row := s.db.QueryRow(
		`SELECT agent_id, session_id, pid, status, started_at, stopped_at
		 FROM pty_sessions WHERE agent_id = ? AND session_id = ?`,
		agentID, sessionID,
	)
	return scanPtySession(row)
}

func (s *sqlPtySessionStore) UpsertPtySession(session *PtySession) error {
	status := session.Status
	if status == "" {
		status = "active"
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pty_sessions (agent_id, session_id, pid, status, started_at, stopped_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		session.AgentID, session.SessionID, session.PID, status, session.StartedAt, session.StoppedAt,
	)
	return err
}

func (s *sqlPtySessionStore) UpdatePtyStatus(agentID, sessionID, status string, stoppedAt *time.Time) error {
	_, err := s.db.Exec(
		`UPDATE pty_sessions SET status = ?, stopped_at = ? WHERE agent_id = ? AND session_id = ?`,
		status, stoppedAt, agentID, sessionID,
	)
	return err
}

func (s *sqlPtySessionStore) ListPtySessions(agentID string) ([]*PtySession, error) {
	rows, err := s.db.Query(
		`SELECT agent_id, session_id, pid, status, started_at, stopped_at
		 FROM pty_sessions WHERE agent_id = ?`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*PtySession
	for rows.Next() {
		s, err := scanPtySessionRow(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func scanPtySession(row *sql.Row) (*PtySession, error) {
	var p PtySession
	var stoppedAt sql.NullTime
	if err := row.Scan(&p.AgentID, &p.SessionID, &p.PID, &p.Status, &p.StartedAt, &stoppedAt); err != nil {
		return nil, err
	}
	if stoppedAt.Valid {
		p.StoppedAt = &stoppedAt.Time
	}
	return &p, nil
}

func scanPtySessionRow(rows *sql.Rows) (*PtySession, error) {
	var p PtySession
	var stoppedAt sql.NullTime
	if err := rows.Scan(&p.AgentID, &p.SessionID, &p.PID, &p.Status, &p.StartedAt, &stoppedAt); err != nil {
		return nil, err
	}
	if stoppedAt.Valid {
		p.StoppedAt = &stoppedAt.Time
	}
	return &p, nil
}
