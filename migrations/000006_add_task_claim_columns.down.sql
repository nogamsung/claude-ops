DROP INDEX idx_tasks_pickup ON tasks;

ALTER TABLE tasks
    DROP COLUMN claimed_at,
    DROP COLUMN worker_id;
