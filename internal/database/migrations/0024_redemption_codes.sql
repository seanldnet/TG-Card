-- Redeemable value, package and product codes, separate from discount promotions.
CREATE TABLE IF NOT EXISTS redemption_codes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  reward_type TEXT NOT NULL CHECK (reward_type IN ('balance','package','product')),
  amount_atomic INTEGER NOT NULL DEFAULT 0,
  target_id TEXT NOT NULL DEFAULT '',
  target_name TEXT NOT NULL DEFAULT '',
  quantity INTEGER NOT NULL DEFAULT 1,
  duration_days INTEGER NOT NULL DEFAULT 0,
  usage_limit INTEGER NOT NULL DEFAULT 1,
  per_user_limit INTEGER NOT NULL DEFAULT 1,
  usage_count INTEGER NOT NULL DEFAULT 0,
  starts_at TEXT NOT NULL DEFAULT '',
  ends_at TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_redemption_codes_active ON redemption_codes(code,status,starts_at,ends_at);
CREATE TABLE IF NOT EXISTS redemption_code_uses (
  id TEXT PRIMARY KEY,
  code_id TEXT NOT NULL REFERENCES redemption_codes(id) ON DELETE CASCADE,
  telegram_user_id TEXT NOT NULL,
  bot_id TEXT NOT NULL DEFAULT '',
  reward_type TEXT NOT NULL,
  reward_value TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_redemption_code_uses_user ON redemption_code_uses(code_id,telegram_user_id);
CREATE TABLE IF NOT EXISTS telegram_package_entitlements (
  id TEXT PRIMARY KEY,
  telegram_user_id TEXT NOT NULL,
  bot_id TEXT NOT NULL DEFAULT '',
  package_name TEXT NOT NULL,
  quantity INTEGER NOT NULL DEFAULT 1,
  granted_days INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL DEFAULT '',
  source_code_id TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_telegram_package_entitlements_user ON telegram_package_entitlements(telegram_user_id,bot_id,expires_at);
