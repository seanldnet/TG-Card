ALTER TABLE orders ADD COLUMN payment_method TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN payment_reference TEXT;
ALTER TABLE orders ADD COLUMN delivery_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN delivery_error TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN delivery_lease_until TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_payment_reference ON orders(payment_reference) WHERE payment_reference IS NOT NULL;
