DROP INDEX idx_tasks_source ON tasks;

ALTER TABLE tasks
    DROP COLUMN maintenance_name,
    DROP COLUMN source;
