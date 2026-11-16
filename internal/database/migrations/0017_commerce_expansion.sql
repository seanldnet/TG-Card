-- TG-Card commerce expansion foundation: stock notifications, quantity pricing, recharge rewards and delivery modes.
ALTER TABLE products ADD COLUMN bulk_discount_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE products ADD COLUMN delivery_config_json TEXT NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS product_stock_notifications (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  telegram_user_id TEXT NOT NULL,
  telegram_chat_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  notified_at TEXT,
  UNIQUE(product_id, bot_id, telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_product_stock_notifications_pending ON product_stock_notifications(product_id, notified_at);

CREATE TABLE IF NOT EXISTS wallet_recharge_bonus_rules (
  id TEXT PRIMARY KEY,
  enabled INTEGER NOT NULL DEFAULT 0,
  tiers_json TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS product_delivery_assets (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  asset_type TEXT NOT NULL,
  value_ciphertext TEXT NOT NULL DEFAULT '',
  value_fingerprint TEXT NOT NULL DEFAULT '',
  telegram_file_id TEXT NOT NULL DEFAULT '',
  one_time INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 0,
  reserved_order_id TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_product_delivery_assets_available ON product_delivery_assets(product_id, asset_type, status, created_at);
