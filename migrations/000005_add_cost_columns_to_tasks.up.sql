ALTER TABLE tasks ADD COLUMN cost_usd                       REAL    NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN total_input_tokens             INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN total_output_tokens            INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN cache_creation_input_tokens    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN cache_read_input_tokens        INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN model_usage_json               TEXT    NOT NULL DEFAULT '{}';
