-- Persist each externally visible delivery step independently.
CREATE TABLE IF NOT EXISTS order_delivery_steps (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL,
  step_key TEXT NOT NULL,
  step_order INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  telegram_message_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(order_id, step_key),
  FOREIGN KEY(order_id) REFERENCES orders(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_order_delivery_steps_order ON order_delivery_steps(order_id, step_order);
CREATE INDEX IF NOT EXISTS idx_order_delivery_steps_status ON order_delivery_steps(status, updated_at);
