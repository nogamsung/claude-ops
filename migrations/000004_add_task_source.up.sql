-- Add source and maintenance_name columns to tasks table.
-- SQLite does not support ADD COLUMN IF NOT EXISTS before version 3.37.0;
-- we guard idempotency at the application level by relying on golang-migrate's
-- versioned schema_migrations table (running this file twice is blocked by the
-- migration framework itself).

ALTER TABLE tasks ADD COLUMN source TEXT NOT NULL DEFAULT 'github_issue';
ALTER TABLE tasks ADD COLUMN maintenance_name TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tasks_source ON tasks(source);
