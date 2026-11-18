CREATE TABLE IF NOT EXISTS upstream_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    provider TEXT NOT NULL DEFAULT 'dujiao_next',
    base_url TEXT NOT NULL DEFAULT '',
    merchant_id TEXT NOT NULL DEFAULT '',
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 0,
    last_test_status TEXT NOT NULL DEFAULT 'not_tested',
    last_test_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    sync_status TEXT NOT NULL DEFAULT 'not_started',
    sync_at TEXT,
    sync_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
