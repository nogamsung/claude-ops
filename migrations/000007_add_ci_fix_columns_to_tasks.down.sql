DROP INDEX uniq_tasks_ci_fix_dedup ON tasks;
DROP INDEX idx_tasks_ci_status     ON tasks;
DROP INDEX idx_tasks_parent        ON tasks;

ALTER TABLE tasks
    DROP COLUMN ci_fix_dedup_key,
    DROP CONSTRAINT chk_tasks_ci_status,
    DROP CONSTRAINT chk_tasks_status,
    ADD CONSTRAINT chk_tasks_status
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    DROP CONSTRAINT chk_tasks_task_type,
    ADD CONSTRAINT chk_tasks_task_type
        CHECK (task_type IN ('feature', 'security', 'perf')),
    DROP COLUMN ci_last_polled_at,
    DROP COLUMN head_sha,
    DROP COLUMN ci_status,
    DROP COLUMN fix_attempt_count,
    DROP COLUMN parent_task_id;
