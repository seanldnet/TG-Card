-- Lease stock-notification delivery so multiple app replicas do not send duplicates.
ALTER TABLE product_stock_notifications ADD COLUMN claim_until TEXT;
CREATE INDEX IF NOT EXISTS idx_product_stock_notifications_claim ON product_stock_notifications(product_id, notified_at, claim_until);
