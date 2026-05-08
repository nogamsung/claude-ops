-- name: ClaimNextTask :execrows
-- Atomically pick one queued task and flip it to running. Skips:
--   * Rows another transaction already locked (FOR UPDATE SKIP LOCKED) —
--     this is what makes parallel workers race-free without an in-process mutex.
--   * Rows whose repo already has a task in 'running' (NOT EXISTS subquery) —
--     this serializes per-repo work even across workers, satisfying the
--     parallel-tasks PRD's "same-repo never concurrent" rule at the SQL
--     layer rather than relying on application-side mutexes.
-- The derived-table wrapping is required because MySQL refuses self-reference
-- in the same UPDATE/SELECT (Error 1093).
UPDATE tasks
SET status     = 'running',
    worker_id  = sqlc.arg(worker_id),
    claimed_at = NOW(3),
    started_at = NOW(3),
    updated_at = NOW(3)
WHERE id = (
    SELECT id FROM (
        SELECT t1.id
        FROM tasks t1
        WHERE t1.status = 'queued'
          AND NOT EXISTS (
              SELECT 1 FROM tasks t2
              WHERE t2.status = 'running'
                AND t2.repo_full_name = t1.repo_full_name
          )
        ORDER BY t1.created_at ASC
        LIMIT 1
        FOR UPDATE SKIP LOCKED
    ) AS picked
);

-- name: GetClaimedTaskID :one
-- Returns the most recently claimed task for a worker. Used right after
-- ClaimNextTask reports rowsAffected=1 to load the row.
SELECT id
FROM tasks
WHERE worker_id = sqlc.arg(worker_id)
  AND status = 'running'
ORDER BY claimed_at DESC
LIMIT 1;

-- name: ReclaimStaleTasks :execrows
-- Periodic safety net: marks tasks orphaned when their worker died without
-- clearing the claim. The cutoff is `now - lease_timeout`; rows still being
-- actively processed have a fresher claimed_at and survive.
UPDATE tasks
SET status     = 'orphaned',
    worker_id  = NULL,
    updated_at = NOW(3)
WHERE status = 'running'
  AND claimed_at IS NOT NULL
  AND claimed_at < sqlc.arg(cutoff);

-- name: ListWatchingTaskIDs :many
-- Returns IDs of tasks the CI watcher should poll. Used by the watcher tick
-- so it can refresh state by calling repo.GetByID, keeping the loaded *Task
-- mapping in one place (the GORM repository).
SELECT id
FROM tasks
WHERE ci_status = 'watching'
ORDER BY ci_last_polled_at IS NULL DESC,  -- never-polled first
         ci_last_polled_at ASC;

-- name: FindFixChildTaskID :one
-- Looks up the existing ci-fix child task for a given (parent, head_sha)
-- pair. Returns ErrNoRows when no duplicate exists, which the caller treats
-- as "free to enqueue".
SELECT id
FROM tasks
WHERE task_type = 'ci-fix'
  AND parent_task_id = sqlc.arg(parent_task_id)
  AND head_sha       = sqlc.arg(head_sha)
LIMIT 1;

-- name: UpdateTaskCIStatus :exec
-- Targeted update used by the CI watcher to advance ci_status / head_sha /
-- ci_last_polled_at without touching the rest of the row.
UPDATE tasks
SET ci_status         = sqlc.arg(ci_status),
    head_sha          = sqlc.arg(head_sha),
    ci_last_polled_at = sqlc.arg(ci_last_polled_at),
    updated_at        = NOW(3)
WHERE id = sqlc.arg(id);
