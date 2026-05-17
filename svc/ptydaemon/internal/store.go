package internal

import (
	"database/sql"
	"errors"
	"fmt"
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

// PTYSessionRecord is a full row from pty_sessions including timestamps.
type PTYSessionRecord struct {
	AgentID   string
	SessionID string
	PID       int
	Status    Status
	StartedAt time.Time
	StoppedAt *time.Time
}

func (s *Store) GetBySessionOnly(sessionID string) (*PTYSessionRecord, error) {
	row := s.db.QueryRow(
		`SELECT agent_id, session_id, pid, status, started_at, stopped_at FROM pty_sessions WHERE session_id = ? LIMIT 1`,
		sessionID,
	)
	return scanRecord(row.Scan)
}

func (s *Store) GetBySession(agentID, sessionID string) (*PTYSessionRecord, error) {
	row := s.db.QueryRow(
		`SELECT agent_id, session_id, pid, status, started_at, stopped_at FROM pty_sessions WHERE agent_id = ? AND session_id = ?`,
		agentID, sessionID,
	)
	return scanRecord(row.Scan)
}

func (s *Store) ListByAgent(agentID string) ([]*PTYSessionRecord, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if agentID == "" {
		rows, err = s.db.Query(
			`SELECT agent_id, session_id, pid, status, started_at, stopped_at FROM pty_sessions ORDER BY started_at DESC`,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT agent_id, session_id, pid, status, started_at, stopped_at FROM pty_sessions WHERE agent_id = ? ORDER BY started_at DESC`,
			agentID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*PTYSessionRecord
	for rows.Next() {
		rec, err := scanRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// MarkAllActiveCrashed marks every session still recorded as active as crashed.
// Called at daemon startup to invalidate sessions that survived a crash/kill.
func (s *Store) MarkAllActiveCrashed() error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`UPDATE pty_sessions SET status = ?, stopped_at = ? WHERE status = ?`,
		string(StatusCrashed), now, string(StatusActive),
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func scanRecord(scan func(...any) error) (*PTYSessionRecord, error) {
	var (
		rec       PTYSessionRecord
		status    string
		startedAt string
		stoppedAt sql.NullString
	)
	if err := scan(&rec.AgentID, &rec.SessionID, &rec.PID, &status, &startedAt, &stoppedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec.Status = Status(status)
	if t, err := parseTime(startedAt); err == nil {
		rec.StartedAt = t
	}
	if stoppedAt.Valid {
		if t, err := parseTime(stoppedAt.String); err == nil {
			rec.StoppedAt = &t
		}
	}
	return &rec, nil
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %q", s)
}
