CREATE TABLE IF NOT EXISTS sandboxes (
    id           TEXT PRIMARY KEY,
    application  TEXT NOT NULL,
    pid          TEXT NOT NULL,
    active       INTEGER NOT NULL,
    created_at   TEXT NOT NULL,
    config_path  TEXT NOT NULL
);
