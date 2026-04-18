CREATE TABLE IF NOT EXISTS task_events (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK(kind IN ('started', 'slack_sent', 'claude_stdout_chunk', 'cancelled', 'usage_warning', 'pr_created', 'failed')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_task_events_task_id ON task_events(task_id);
CREATE INDEX IF NOT EXISTS idx_task_events_created_at ON task_events(created_at);
