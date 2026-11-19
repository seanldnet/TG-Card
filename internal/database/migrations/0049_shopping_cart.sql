-- Persistent Telegram shopping carts, isolated by Bot and Telegram account.
CREATE TABLE IF NOT EXISTS shopping_cart_items (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 100),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(bot_id, telegram_user_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_shopping_cart_user
  ON shopping_cart_items(bot_id, telegram_user_id, updated_at);
