CREATE TABLE IF NOT EXISTS bots (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    username TEXT NOT NULL,
    token_ciphertext TEXT NOT NULL,
    token_fingerprint TEXT NOT NULL UNIQUE,
    status INTEGER NOT NULL DEFAULT 1,
    welcome_text TEXT NOT NULL DEFAULT '',
    menu_config TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_bots_status_created ON bots(status, created_at);
