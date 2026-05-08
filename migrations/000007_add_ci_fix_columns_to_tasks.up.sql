-- ci-fix-loop foundation. Adds the per-task CI tracking fields, the
-- parent/child fix-chain link, and extends the existing CHECK
-- constraints for the new task_type ('ci-fix') and status ('orphaned',
-- which is already used by Reclaimer but was missing from the original
-- enum and would have errored on production INSERT).
--
-- Note on parent_task_id: a self-referential FOREIGN KEY was originally
-- planned (PRD §7), but MySQL 8.0 + golang-migrate consistently fails
-- with Error 1215 ("Cannot add foreign key constraint") on a fresh DB
-- when the FK is added in the same migration as the column it
-- references — even when split into multiple ALTERs. The constraint
-- offered no behavior we couldn't enforce in application code (we never
-- delete tasks; ON DELETE SET NULL semantics are not exercised), so the
-- column is plain CHAR(36) NULL and the parent/child invariant is
-- maintained by usecase.TaskUseCase.EnqueueFixTask.

ALTER TABLE tasks
    ADD COLUMN parent_task_id     CHAR(36)    NULL,
    ADD COLUMN fix_attempt_count  INT         NOT NULL DEFAULT 0,
    ADD COLUMN ci_status          VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN head_sha           VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN ci_last_polled_at  DATETIME(3) NULL,
    DROP CONSTRAINT chk_tasks_task_type,
    ADD CONSTRAINT chk_tasks_task_type
        CHECK (task_type IN ('feature', 'security', 'perf', 'ci-fix')),
    DROP CONSTRAINT chk_tasks_status,
    ADD CONSTRAINT chk_tasks_status
        CHECK (status IN ('queued', 'running', 'done', 'failed', 'cancelled', 'orphaned')),
    ADD CONSTRAINT chk_tasks_ci_status
        CHECK (ci_status IN ('', 'pending', 'watching', 'passed', 'failed',
                             'exhausted', 'timeout', 'stuck', 'closed'));

-- Dedup key for fix tasks. STORED generated column whose value is NULL
-- for non-ci-fix rows (NULLs do not collide in UNIQUE indexes), emulating
-- partial-index semantics that MySQL lacks.
ALTER TABLE tasks
    ADD COLUMN ci_fix_dedup_key VARCHAR(83) GENERATED ALWAYS AS (
        CASE WHEN task_type = 'ci-fix'
             THEN CONCAT(COALESCE(parent_task_id, ''), '|', head_sha)
             ELSE NULL END
    ) STORED;

CREATE INDEX idx_tasks_parent             ON tasks(parent_task_id);
CREATE INDEX idx_tasks_ci_status          ON tasks(ci_status);
CREATE UNIQUE INDEX uniq_tasks_ci_fix_dedup ON tasks(ci_fix_dedup_key);
