ALTER TABLE tasks DROP COLUMN model_usage_json;
ALTER TABLE tasks DROP COLUMN cache_read_input_tokens;
ALTER TABLE tasks DROP COLUMN cache_creation_input_tokens;
ALTER TABLE tasks DROP COLUMN total_output_tokens;
ALTER TABLE tasks DROP COLUMN total_input_tokens;
ALTER TABLE tasks DROP COLUMN cost_usd;
