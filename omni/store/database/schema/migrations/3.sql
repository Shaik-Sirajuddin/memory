ALTER TABLE workspaces ADD COLUMN remote TEXT NOT NULL DEFAULT 'localhost';
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_remote_name ON workspaces(remote, name);
