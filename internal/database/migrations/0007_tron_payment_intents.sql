ALTER TABLE orders ADD COLUMN payment_amount_atomic BIGINT NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_pending_payment_amount ON orders(payment_amount_atomic) WHERE status='pending_payment' AND payment_amount_atomic > 0;
CREATE INDEX IF NOT EXISTS idx_orders_pending_payment_expiry ON orders(status, expires_at);
