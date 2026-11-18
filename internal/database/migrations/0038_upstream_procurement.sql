-- Durable Dujiao-Next procurement queue, upstream order mapping and callback idempotency.
CREATE TABLE IF NOT EXISTS upstream_procurements (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL UNIQUE,
  order_no TEXT NOT NULL UNIQUE,
  product_id TEXT NOT NULL,
  upstream_product_id TEXT NOT NULL,
  upstream_sku_id TEXT NOT NULL,
  quantity INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  upstream_order_id TEXT UNIQUE,
  upstream_order_no TEXT NOT NULL DEFAULT '',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  poll_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TEXT NOT NULL,
  deadline_at TEXT NOT NULL,
  last_error TEXT NOT NULL DEFAULT '',
  fulfillment_ciphertext TEXT NOT NULL DEFAULT '',
  delivered_at TEXT NOT NULL DEFAULT '',
  refunded_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_upstream_procurements_due ON upstream_procurements(status,next_attempt_at);
CREATE INDEX IF NOT EXISTS idx_upstream_procurements_order_no ON upstream_procurements(order_no,upstream_order_no);
CREATE TABLE IF NOT EXISTS upstream_procurement_events (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  detail TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_upstream_procurement_events_order ON upstream_procurement_events(order_id,created_at DESC);
CREATE TABLE IF NOT EXISTS upstream_callback_events (
  id TEXT PRIMARY KEY,
  event_key TEXT NOT NULL UNIQUE,
  downstream_order_no TEXT NOT NULL,
  upstream_order_id TEXT NOT NULL,
  status TEXT NOT NULL,
  received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_upstream_callback_events_order ON upstream_callback_events(downstream_order_no,received_at DESC);
ALTER TABLE upstream_settings ADD COLUMN callback_url TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_settings ADD COLUMN purchase_poll_interval_seconds INTEGER NOT NULL DEFAULT 15;
ALTER TABLE upstream_settings ADD COLUMN purchase_timeout_minutes INTEGER NOT NULL DEFAULT 30;
ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_balance TEXT NOT NULL DEFAULT '';
ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE daily_reconciliation_reports ADD COLUMN supplier_balance_checked_at TEXT NOT NULL DEFAULT '';
ALTER TABLE upstream_procurements ADD COLUMN delivery_lease_until TEXT NOT NULL DEFAULT '';
