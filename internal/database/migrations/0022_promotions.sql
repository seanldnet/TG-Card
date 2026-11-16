-- Promotions, redeemable coupon codes and order redemption accounting.
ALTER TABLE orders ADD COLUMN promotion_id TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN promotion_code TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN promotion_discount_minor INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS promotions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  code TEXT NOT NULL UNIQUE,
  discount_type TEXT NOT NULL CHECK (discount_type IN ('fixed','percent')),
  discount_value INTEGER NOT NULL,
  minimum_order_minor INTEGER NOT NULL DEFAULT 0,
  maximum_discount_minor INTEGER NOT NULL DEFAULT 0,
  starts_at TEXT NOT NULL DEFAULT '',
  ends_at TEXT NOT NULL DEFAULT '',
  usage_limit INTEGER NOT NULL DEFAULT 0,
  per_user_limit INTEGER NOT NULL DEFAULT 1,
  usage_count INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_promotions_active_code ON promotions(code,status,starts_at,ends_at);

CREATE TABLE IF NOT EXISTS promotion_redemptions (
  id TEXT PRIMARY KEY,
  promotion_id TEXT NOT NULL REFERENCES promotions(id) ON DELETE CASCADE,
  order_id TEXT NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
  telegram_user_id TEXT NOT NULL,
  discount_minor INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'reserved' CHECK (status IN ('reserved','redeemed','released')),
  created_at TEXT NOT NULL,
  redeemed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_promotion_redemptions_user ON promotion_redemptions(promotion_id,telegram_user_id,status);
