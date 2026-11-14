CREATE TABLE IF NOT EXISTS orders (
    id TEXT PRIMARY KEY,
    order_no TEXT NOT NULL UNIQUE,
    bot_id TEXT NOT NULL DEFAULT '',
    telegram_user_id TEXT NOT NULL,
    telegram_chat_id TEXT NOT NULL,
    product_id TEXT,
    product_name TEXT NOT NULL,
    unit_price_minor INTEGER NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    amount_minor INTEGER NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USDT',
    status TEXT NOT NULL DEFAULT 'pending_payment',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    paid_at TEXT,
    delivered_at TEXT,
    FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_orders_status_created ON orders(status, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_telegram_user ON orders(telegram_user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_orders_bot_created ON orders(bot_id, created_at);
