ALTER TABLE workspaces ADD COLUMN remote TEXT NOT NULL DEFAULT 'localhost';
UPDATE workspaces
    SET name = name || '_' || hex(randomblob(4))
    WHERE rowid NOT IN (
        SELECT MIN(rowid) FROM workspaces GROUP BY remote, name
    );
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_remote_name ON workspaces(remote, name);
