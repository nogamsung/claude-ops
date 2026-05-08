CREATE TABLE IF NOT EXISTS tasks (
    id                       CHAR(36)        NOT NULL,
    repo_full_name           VARCHAR(255)    NOT NULL,
    issue_number             BIGINT          NOT NULL,
    issue_title              VARCHAR(500)    NOT NULL DEFAULT '',
    task_type                VARCHAR(16)     NOT NULL DEFAULT 'feature',
    status                   VARCHAR(16)     NOT NULL DEFAULT 'queued',
    prompt_template          MEDIUMTEXT      NOT NULL,
    worktree_path            VARCHAR(512)    NOT NULL DEFAULT '',
    pr_url                   VARCHAR(500)    NOT NULL DEFAULT '',
    pr_number                BIGINT          NOT NULL DEFAULT 0,
    started_at               DATETIME(3)     NULL,
    finished_at              DATETIME(3)     NULL,
    estimated_input_tokens   BIGINT          NOT NULL DEFAULT 0,
    estimated_output_tokens  BIGINT          NOT NULL DEFAULT 0,
    exit_code                INT             NULL,
    stderr_tail              MEDIUMTEXT      NOT NULL,
    created_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at               DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT chk_tasks_task_type CHECK (task_type IN ('feature', 'security', 'perf')),
    CONSTRAINT chk_tasks_status    CHECK (status    IN ('queued', 'running', 'done', 'failed', 'cancelled'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_tasks_status     ON tasks(status);
CREATE INDEX idx_tasks_repo_issue ON tasks(repo_full_name, issue_number);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
