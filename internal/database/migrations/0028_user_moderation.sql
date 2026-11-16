ALTER TABLE telegram_users ADD COLUMN is_banned INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_telegram_users_banned ON telegram_users(is_banned, updated_at DESC);
