CREATE TABLE IF NOT EXISTS telegram_users (
  id TEXT PRIMARY KEY,
  telegram_user_id TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL DEFAULT '',
  first_name TEXT NOT NULL DEFAULT '',
  last_name TEXT NOT NULL DEFAULT '',
  last_seen_bot_id TEXT NOT NULL DEFAULT '',
  balance_atomic BIGINT NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_telegram_users_updated ON telegram_users(updated_at DESC);
CREATE TABLE IF NOT EXISTS wallet_transactions (
  id TEXT PRIMARY KEY,
  telegram_user_id TEXT NOT NULL,
  admin_id TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  amount_atomic BIGINT NOT NULL,
  balance_after_atomic BIGINT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_time ON wallet_transactions(telegram_user_id,created_at DESC);
