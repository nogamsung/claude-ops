-- ci-fix-loop foundation. Adds the per-task CI tracking fields, the
-- parent/child fix-chain link, and extends the existing CHECK
-- constraints for the new task_type ('ci-fix') and status ('orphaned',
-- which is already used by Reclaimer but was missing from the original
-- enum and would have errored on production INSERT).

ALTER TABLE tasks
    ADD COLUMN parent_task_id     CHAR(36)    NULL,
    ADD COLUMN fix_attempt_count  INT         NOT NULL DEFAULT 0,
    ADD COLUMN ci_status          VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN head_sha           VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN ci_last_polled_at  DATETIME(3) NULL,
    ADD CONSTRAINT fk_tasks_parent
        FOREIGN KEY (parent_task_id) REFERENCES tasks(id) ON DELETE SET NULL,
    DROP CONSTRAINT chk_tasks_task_type,
    ADD CONSTRAINT chk_tasks_task_type
        CHECK (task_type IN ('feature', 'security', 'perf', 'ci-fix')),
    DROP CONSTRAINT chk_tasks_status,
    ADD CONSTRAINT chk_tasks_status
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled', 'orphaned')),
    ADD CONSTRAINT chk_tasks_ci_status
        CHECK (ci_status IN ('', 'pending', 'watching', 'passed', 'failed',
                             'exhausted', 'timeout', 'stuck', 'closed'));

-- Dedup index for fix tasks: at most one ci-fix task per (parent, head_sha).
-- MySQL has no partial-index syntax, so we use a STORED generated column
-- that is NULL for non-ci-fix rows (NULLs do not collide in UNIQUE indexes).
ALTER TABLE tasks
    ADD COLUMN ci_fix_dedup_key VARCHAR(83) GENERATED ALWAYS AS (
        CASE WHEN task_type = 'ci-fix'
             THEN CONCAT(COALESCE(parent_task_id, ''), '|', head_sha)
             ELSE NULL END
    ) STORED;

CREATE INDEX idx_tasks_parent             ON tasks(parent_task_id);
CREATE INDEX idx_tasks_ci_status          ON tasks(ci_status);
CREATE UNIQUE INDEX uniq_tasks_ci_fix_dedup ON tasks(ci_fix_dedup_key);
