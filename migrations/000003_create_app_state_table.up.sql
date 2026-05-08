CREATE TABLE IF NOT EXISTS app_states (
    `key`      VARCHAR(64)     NOT NULL,
    value_json JSON            NOT NULL,
    updated_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT IGNORE INTO app_states (`key`, value_json) VALUES ('full_mode',    JSON_OBJECT('enabled', FALSE));
INSERT IGNORE INTO app_states (`key`, value_json) VALUES ('last_poll_at', JSON_OBJECT('timestamp', NULL));
