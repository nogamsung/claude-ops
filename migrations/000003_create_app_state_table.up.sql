CREATE TABLE IF NOT EXISTS app_states (
    key        TEXT PRIMARY KEY,
    value_json TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO app_states (key, value_json) VALUES ('full_mode', '{"enabled": false}');
INSERT OR IGNORE INTO app_states (key, value_json) VALUES ('last_poll_at', '{"timestamp": null}');
