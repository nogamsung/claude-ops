ALTER TABLE tasks
    DROP COLUMN model_usage_json,
    DROP COLUMN cache_read_input_tokens,
    DROP COLUMN cache_creation_input_tokens,
    DROP COLUMN total_output_tokens,
    DROP COLUMN total_input_tokens,
    DROP COLUMN cost_usd;
