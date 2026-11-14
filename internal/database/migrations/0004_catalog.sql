CREATE TABLE IF NOT EXISTS product_categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    status INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_categories_name ON product_categories(name);

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    category_id TEXT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_minor INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'USDT',
    delivery_type TEXT NOT NULL DEFAULT 'card',
    status INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(category_id) REFERENCES product_categories(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_products_category_status ON products(category_id, status);

CREATE TABLE IF NOT EXISTS product_keys (
    id TEXT PRIMARY KEY,
    product_id TEXT NOT NULL,
    secret_ciphertext TEXT NOT NULL,
    secret_fingerprint TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 0,
    order_id TEXT,
    created_at TEXT NOT NULL,
    sold_at TEXT,
    FOREIGN KEY(product_id) REFERENCES products(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_keys_unique ON product_keys(product_id, secret_fingerprint);
CREATE INDEX IF NOT EXISTS idx_product_keys_stock ON product_keys(product_id, status);
