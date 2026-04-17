CREATE TABLE IF NOT EXISTS workspaces (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    workspace_dir TEXT NOT NULL UNIQUE
);
