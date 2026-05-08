-- worker_id  : opaque token identifying the worker (or instance) that holds
--              the task. Set by ClaimNextTask, cleared on terminal status.
-- claimed_at : DATETIME(3) of the claim. Together with cfg.lease_timeout this
--              lets a periodic reclaimer recover orphaned rows whose worker
--              died.
-- The composite (status, created_at) index supports the FOR UPDATE SKIP LOCKED
-- pickup query. Existing idx_tasks_status remains for snapshot/list queries.
ALTER TABLE tasks
    ADD COLUMN worker_id  VARCHAR(64) NULL,
    ADD COLUMN claimed_at DATETIME(3) NULL;

CREATE INDEX idx_tasks_pickup ON tasks(status, created_at);
