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

-- Existing uncertain file-card orders predate per-step tracking. Preserve their ambiguity
-- by creating two uncertain steps so an operator must verify each external side effect.
INSERT INTO order_delivery_steps (
  id,order_id,step_key,step_order,status,attempts,last_error,started_at,created_at,updated_at
)
SELECT
  'legacy-file-card:' || id || ':file',id,'file',1,'uncertain',
  CASE WHEN delivery_attempts > 0 THEN delivery_attempts ELSE 1 END,
  COALESCE(delivery_error,'legacy file delivery outcome requires manual verification'),
  updated_at,created_at,updated_at
FROM orders
WHERE status='delivery_uncertain' AND delivery_type='file_card'
ON CONFLICT(order_id,step_key) DO NOTHING;

INSERT INTO order_delivery_steps (
  id,order_id,step_key,step_order,status,attempts,last_error,started_at,created_at,updated_at
)
SELECT
  'legacy-file-card:' || id || ':card_message',id,'card_message',2,'uncertain',
  CASE WHEN delivery_attempts > 0 THEN delivery_attempts ELSE 1 END,
  COALESCE(delivery_error,'legacy card message delivery outcome requires manual verification'),
  updated_at,created_at,updated_at
FROM orders
WHERE status='delivery_uncertain' AND delivery_type='file_card'
ON CONFLICT(order_id,step_key) DO NOTHING;
