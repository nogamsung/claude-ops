CREATE TABLE IF NOT EXISTS task_events (
    id           CHAR(36)        NOT NULL,
    task_id      CHAR(36)        NOT NULL,
    kind         VARCHAR(32)     NOT NULL,
    payload_json JSON            NOT NULL,
    created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT chk_task_events_kind CHECK (kind IN ('started', 'slack_sent', 'claude_stdout_chunk', 'cancelled', 'usage_warning', 'pr_created', 'failed')),
    CONSTRAINT fk_task_events_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_task_events_task_id    ON task_events(task_id);
CREATE INDEX idx_task_events_created_at ON task_events(created_at);
