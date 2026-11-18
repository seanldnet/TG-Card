-- Dujiao-Next purchase lifecycle. Buyer payment records are never rewritten by this queue.
ALTER TABLE orders ADD COLUMN upstream_source_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN upstream_product_id_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN upstream_sku_id_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN upstream_fulfillment_type_snapshot TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN procurement_error TEXT NOT NULL DEFAULT '';

ALTER TABLE upstream_settings ADD COLUMN procurement_poll_interval_seconds INTEGER NOT NULL DEFAULT 30;
ALTER TABLE upstream_settings ADD COLUMN procurement_timeout_minutes INTEGER NOT NULL DEFAULT 30;
ALTER TABLE upstream_settings ADD COLUMN supplier_low_balance_threshold TEXT NOT NULL DEFAULT '0';
ALTER TABLE upstream_settings ADD COLUMN callback_url TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS upstream_procurements (
  local_order_id TEXT PRIMARY KEY,
  local_order_no TEXT NOT NULL UNIQUE,
  external_order_id TEXT NOT NULL DEFAULT '',
  external_order_no TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  request_attempts INTEGER NOT NULL DEFAULT 0,
  next_poll_at TEXT,
  deadline_at TEXT NOT NULL,
  last_request_at TEXT,
  lease_until TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  payload_ciphertext TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(local_order_id) REFERENCES orders(id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_procurements_external_id
  ON upstream_procurements(external_order_id) WHERE external_order_id <> '';
CREATE INDEX IF NOT EXISTS idx_upstream_procurements_due
  ON upstream_procurements(status,next_poll_at,deadline_at);

CREATE TABLE IF NOT EXISTS upstream_procurement_events (
  id TEXT PRIMARY KEY,
  local_order_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT 'system',
  created_at TEXT NOT NULL,
  FOREIGN KEY(local_order_id) REFERENCES orders(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_upstream_procurement_events_order
  ON upstream_procurement_events(local_order_id,created_at DESC);

CREATE TABLE IF NOT EXISTS upstream_callback_events (
  payload_hash TEXT PRIMARY KEY,
  local_order_no TEXT NOT NULL,
  event_type TEXT NOT NULL DEFAULT '',
  result TEXT NOT NULL DEFAULT '',
  received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_upstream_callback_events_order
  ON upstream_callback_events(local_order_no,received_at DESC);

CREATE TABLE IF NOT EXISTS upstream_procurement_refunds (
  order_id TEXT PRIMARY KEY,
  wallet_transaction_id TEXT NOT NULL UNIQUE,
  amount_atomic BIGINT NOT NULL,
  admin_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS upstream_balance_snapshots (
  id TEXT PRIMARY KEY,
  balance TEXT NOT NULL,
  currency TEXT NOT NULL,
  checked_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_upstream_balance_snapshots_time
  ON upstream_balance_snapshots(checked_at DESC);

ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_balance TEXT NOT NULL DEFAULT '';
ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_balance_checked_at TEXT NOT NULL DEFAULT '';
