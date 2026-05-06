CREATE TABLE IF NOT EXISTS code_sessions (
    id               TEXT PRIMARY KEY,
    agent_id         TEXT NOT NULL,
    model_provider   TEXT NOT NULL DEFAULT '',
    model_name       TEXT NOT NULL DEFAULT '',
    idx              INTEGER NOT NULL DEFAULT 0,
    is_active        INTEGER NOT NULL DEFAULT 0,
    prompts          INTEGER NOT NULL DEFAULT 0,
    last_sync_prompt INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id)
);
