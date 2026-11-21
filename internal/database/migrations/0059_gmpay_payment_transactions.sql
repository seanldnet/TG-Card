CREATE TABLE IF NOT EXISTS gmpay_payment_transactions (
  id TEXT PRIMARY KEY,
  pid TEXT NOT NULL,
  trade_id TEXT NOT NULL,
  order_id TEXT NOT NULL,
  amount TEXT NOT NULL,
  actual_amount TEXT NOT NULL,
  token TEXT NOT NULL DEFAULT '',
  block_transaction_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  reconciliation_note TEXT NOT NULL DEFAULT '',
  callback_payload TEXT NOT NULL DEFAULT '',
  received_at TEXT NOT NULL,
  processed_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(trade_id),
  UNIQUE(order_id)
);
CREATE INDEX IF NOT EXISTS idx_gmpay_transactions_status_time
  ON gmpay_payment_transactions(status, received_at);
