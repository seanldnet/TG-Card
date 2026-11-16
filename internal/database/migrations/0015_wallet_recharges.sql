CREATE TABLE IF NOT EXISTS wallet_recharges (
  id TEXT PRIMARY KEY,
  recharge_no TEXT NOT NULL UNIQUE,
  bot_id TEXT NOT NULL DEFAULT '',
  telegram_user_id TEXT NOT NULL,
  telegram_chat_id TEXT NOT NULL,
  amount_atomic BIGINT NOT NULL,
  payment_amount_atomic BIGINT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending_payment',
  payment_reference TEXT,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  paid_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_recharges_payment_reference ON wallet_recharges(payment_reference) WHERE payment_reference IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_recharges_pending_amount ON wallet_recharges(payment_amount_atomic) WHERE status='pending_payment';
CREATE INDEX IF NOT EXISTS idx_wallet_recharges_user_created ON wallet_recharges(telegram_user_id,created_at DESC);
