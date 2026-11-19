CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_sync_single_active_job ON upstream_sync_jobs((1)) WHERE status IN ('queued','running','cleaning');
