CREATE TABLE IF NOT EXISTS upstream_sync_jobs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    total_items INTEGER NOT NULL DEFAULT 0,
    processed_items INTEGER NOT NULL DEFAULT 0,
    current_page INTEGER NOT NULL DEFAULT 0,
    batch_size INTEGER NOT NULL DEFAULT 25,
    delay_ms INTEGER NOT NULL DEFAULT 1000,
    error_count INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_upstream_sync_jobs_status_updated ON upstream_sync_jobs(status, updated_at);