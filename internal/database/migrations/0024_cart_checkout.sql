-- One payment invoice for a homogeneous Telegram cart checkout.
CREATE TABLE IF NOT EXISTS cart_checkout_items (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id TEXT NOT NULL,
  product_name TEXT NOT NULL,
  quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
  unit_price_minor INTEGER NOT NULL,
  line_total_minor INTEGER NOT NULL,
  currency TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cart_checkout_items_order ON cart_checkout_items(order_id, created_at);
