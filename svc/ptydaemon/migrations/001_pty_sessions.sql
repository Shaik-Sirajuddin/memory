CREATE TABLE IF NOT EXISTS pty_sessions (
    agent_id    TEXT     NOT NULL,
    session_id  TEXT     NOT NULL,
    pid         INTEGER  NOT NULL,
    status      TEXT     NOT NULL DEFAULT 'active',
    started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    stopped_at  DATETIME,
    PRIMARY KEY (agent_id, session_id)
);
