-- Snapshot fulfillment settings at checkout so later product edits cannot alter paid orders.
ALTER TABLE orders ADD COLUMN delivery_type TEXT NOT NULL DEFAULT 'card';
ALTER TABLE orders ADD COLUMN delivery_config_snapshot TEXT NOT NULL DEFAULT '{}';
ALTER TABLE orders ADD COLUMN delivery_payload_json TEXT NOT NULL DEFAULT '{}';
