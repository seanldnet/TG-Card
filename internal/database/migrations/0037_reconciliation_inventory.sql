-- Daily reconciliation snapshots and read-only wallet balance audit history.
ALTER TABLE orders ADD COLUMN delivery_failed_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS daily_reconciliation_reports (
  report_date TEXT PRIMARY KEY,
  revenue_atomic BIGINT NOT NULL DEFAULT 0,
  order_count BIGINT NOT NULL DEFAULT 0,
  delivered_count BIGINT NOT NULL DEFAULT 0,
  delivery_failed_count BIGINT NOT NULL DEFAULT 0,
  refund_atomic BIGINT NOT NULL DEFAULT 0,
  recharge_atomic BIGINT NOT NULL DEFAULT 0,
  unmatched_transaction_count BIGINT NOT NULL DEFAULT 0,
  manual_review_transaction_count BIGINT NOT NULL DEFAULT 0,
  generated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_daily_reconciliation_generated ON daily_reconciliation_reports(generated_at DESC);

CREATE TABLE IF NOT EXISTS balance_reconciliation_runs (
  run_key TEXT PRIMARY KEY,
  run_at TEXT NOT NULL,
  checked_users BIGINT NOT NULL DEFAULT 0,
  mismatch_users BIGINT NOT NULL DEFAULT 0,
  total_difference_atomic BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_balance_reconciliation_runs_time ON balance_reconciliation_runs(run_at DESC);

CREATE TABLE IF NOT EXISTS balance_reconciliation_anomalies (
  run_key TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  recorded_balance_atomic BIGINT NOT NULL,
  ledger_balance_atomic BIGINT NOT NULL,
  difference_atomic BIGINT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(run_key, telegram_user_id),
  FOREIGN KEY(run_key) REFERENCES balance_reconciliation_runs(run_key) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_balance_reconciliation_anomalies_user ON balance_reconciliation_anomalies(telegram_user_id, created_at DESC);
