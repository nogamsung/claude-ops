ALTER TABLE tasks
    ADD COLUMN source            VARCHAR(32)  NOT NULL DEFAULT 'github_issue',
    ADD COLUMN maintenance_name  VARCHAR(128) NOT NULL DEFAULT '';

CREATE INDEX idx_tasks_source ON tasks(source);
