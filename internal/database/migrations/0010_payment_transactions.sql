CREATE TABLE IF NOT EXISTS payment_transactions (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  transaction_id TEXT NOT NULL,
  from_address TEXT NOT NULL DEFAULT '',
  to_address TEXT NOT NULL DEFAULT '',
  amount_atomic BIGINT NOT NULL,
  token_decimals INTEGER NOT NULL DEFAULT 6,
  block_timestamp_ms BIGINT NOT NULL,
  status TEXT NOT NULL DEFAULT 'unmatched',
  order_id TEXT,
  reconciliation_note TEXT NOT NULL DEFAULT '',
  first_seen_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider, transaction_id)
);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_status_time ON payment_transactions(status, block_timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS idx_payment_transactions_order ON payment_transactions(order_id);
