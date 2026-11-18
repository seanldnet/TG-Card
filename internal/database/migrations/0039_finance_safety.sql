-- PR D: supplier purchase debit reconciliation, refund ledger integrity, and unified finance exceptions.
ALTER TABLE upstream_procurements ADD COLUMN supplier_balance_before TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN supplier_balance_after TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN supplier_expected_debit TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN supplier_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN supplier_fund_check_status TEXT NOT NULL DEFAULT 'not_checked';
ALTER TABLE upstream_procurements ADD COLUMN supplier_fund_difference TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN supplier_fund_checked_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS upstream_procurement_fund_checks (
  local_order_id TEXT PRIMARY KEY,
  local_order_no TEXT NOT NULL UNIQUE,
  external_order_id TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT '',
  balance_before TEXT NOT NULL DEFAULT '',
  balance_after TEXT NOT NULL DEFAULT '',
  expected_debit TEXT NOT NULL DEFAULT '',
  observed_debit TEXT NOT NULL DEFAULT '',
  difference TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  detail TEXT NOT NULL DEFAULT '',
  checked_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(local_order_id) REFERENCES upstream_procurements(local_order_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_upstream_fund_checks_status ON upstream_procurement_fund_checks(status,updated_at DESC);

CREATE TABLE IF NOT EXISTS finance_exceptions (
  id TEXT PRIMARY KEY,
  source_key TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'warning',
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  title TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  resolution_note TEXT NOT NULL DEFAULT '',
  resolved_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_finance_exceptions_status ON finance_exceptions(status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_finance_exceptions_category ON finance_exceptions(category,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS finance_exception_alerts (
  alert_key TEXT NOT NULL,
  alert_date TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(alert_key,alert_date)
);
