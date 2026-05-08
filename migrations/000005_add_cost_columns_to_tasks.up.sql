ALTER TABLE tasks
    ADD COLUMN cost_usd                    DECIMAL(12,6) NOT NULL DEFAULT 0,
    ADD COLUMN total_input_tokens          BIGINT        NOT NULL DEFAULT 0,
    ADD COLUMN total_output_tokens         BIGINT        NOT NULL DEFAULT 0,
    ADD COLUMN cache_creation_input_tokens BIGINT        NOT NULL DEFAULT 0,
    ADD COLUMN cache_read_input_tokens     BIGINT        NOT NULL DEFAULT 0,
    ADD COLUMN model_usage_json            JSON          NOT NULL;
