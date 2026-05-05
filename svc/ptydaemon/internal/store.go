package internal

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS pty_sessions (
    agent_id    TEXT     NOT NULL,
    session_id  TEXT     NOT NULL,
    pid         INTEGER  NOT NULL,
    status      TEXT     NOT NULL DEFAULT 'active',
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    stopped_at  DATETIME,
    PRIMARY KEY (agent_id, session_id)
);`

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Insert(info *PTYTerminalInfo) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pty_sessions (agent_id, session_id, pid, status) VALUES (?, ?, ?, ?)`,
		info.AgentID, info.SessionID, info.PID, string(info.Status),
	)
	return err
}

func (s *Store) UpdateStatus(agentID, sessionID string, status Status) error {
	var stoppedAt *time.Time
	if status != StatusActive {
		now := time.Now().UTC()
		stoppedAt = &now
	}
	_, err := s.db.Exec(
		`UPDATE pty_sessions SET status = ?, stopped_at = ? WHERE agent_id = ? AND session_id = ?`,
		string(status), stoppedAt, agentID, sessionID,
	)
	return err
}

func (s *Store) List() ([]*PTYTerminalInfo, error) {
	rows, err := s.db.Query(`SELECT agent_id, session_id, pid, status FROM pty_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var infos []*PTYTerminalInfo
	for rows.Next() {
		info := &PTYTerminalInfo{}
		var status string
		if err := rows.Scan(&info.AgentID, &info.SessionID, &info.PID, &status); err != nil {
			return nil, err
		}
		info.Status = Status(status)
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

func (s *Store) Close() error {
	return s.db.Close()
}
