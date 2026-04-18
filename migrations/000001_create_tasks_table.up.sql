CREATE TABLE IF NOT EXISTS tasks (
    id                       TEXT PRIMARY KEY,
    repo_full_name           TEXT NOT NULL,
    issue_number             INTEGER NOT NULL,
    issue_title              TEXT NOT NULL DEFAULT '',
    task_type                TEXT NOT NULL DEFAULT 'feature' CHECK(task_type IN ('feature', 'security', 'perf')),
    status                   TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    prompt_template          TEXT NOT NULL DEFAULT '',
    worktree_path            TEXT NOT NULL DEFAULT '',
    pr_url                   TEXT NOT NULL DEFAULT '',
    pr_number                INTEGER NOT NULL DEFAULT 0,
    started_at               DATETIME,
    finished_at              DATETIME,
    estimated_input_tokens   INTEGER NOT NULL DEFAULT 0,
    estimated_output_tokens  INTEGER NOT NULL DEFAULT 0,
    exit_code                INTEGER,
    stderr_tail              TEXT NOT NULL DEFAULT '',
    created_at               DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at               DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_repo_issue ON tasks(repo_full_name, issue_number);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
