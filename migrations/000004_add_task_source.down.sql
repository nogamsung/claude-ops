-- SQLite does not support DROP COLUMN before version 3.35.0.
-- We recreate the table without the added columns.

DROP INDEX IF EXISTS idx_tasks_source;

CREATE TABLE IF NOT EXISTS tasks_backup AS SELECT
    id, repo_full_name, issue_number, issue_title, task_type, status,
    prompt_template, worktree_path, pr_url, pr_number,
    started_at, finished_at,
    estimated_input_tokens, estimated_output_tokens,
    exit_code, stderr_tail,
    created_at, updated_at
FROM tasks;

DROP TABLE tasks;

ALTER TABLE tasks_backup RENAME TO tasks;
