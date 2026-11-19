ALTER TABLE upstream_settings ADD COLUMN sync_batch_size INTEGER NOT NULL DEFAULT 25;
ALTER TABLE upstream_settings ADD COLUMN sync_batch_delay_ms INTEGER NOT NULL DEFAULT 1000;
