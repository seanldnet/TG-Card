ALTER TABLE product_categories ADD COLUMN source_type TEXT NOT NULL DEFAULT 'local';
ALTER TABLE product_categories ADD COLUMN upstream_category_id TEXT;
ALTER TABLE product_categories ADD COLUMN upstream_parent_id TEXT;
ALTER TABLE product_categories ADD COLUMN upstream_sync_batch_id TEXT;

ALTER TABLE products ADD COLUMN source_type TEXT NOT NULL DEFAULT 'local';
ALTER TABLE products ADD COLUMN upstream_product_id TEXT;
ALTER TABLE products ADD COLUMN upstream_sku_id TEXT;
ALTER TABLE products ADD COLUMN upstream_slug TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN upstream_price_snapshot_minor INTEGER;
ALTER TABLE products ADD COLUMN upstream_currency TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN upstream_stock_quantity INTEGER NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN upstream_stock_status TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN image_paths_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE products ADD COLUMN upstream_sync_batch_id TEXT;

DROP INDEX IF EXISTS idx_product_categories_name;
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_categories_local_name ON product_categories(name) WHERE source_type = 'local';
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_categories_upstream_id ON product_categories(upstream_category_id) WHERE source_type = 'upstream' AND upstream_category_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_upstream_sku ON products(upstream_product_id, upstream_sku_id) WHERE source_type = 'upstream' AND upstream_product_id IS NOT NULL AND upstream_sku_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_products_source_status ON products(source_type, status);
CREATE INDEX IF NOT EXISTS idx_categories_source_status ON product_categories(source_type, status);

ALTER TABLE upstream_settings ADD COLUMN markup_bps INTEGER NOT NULL DEFAULT 0;
ALTER TABLE upstream_settings ADD COLUMN markup_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE upstream_settings ADD COLUMN sync_interval_minutes INTEGER NOT NULL DEFAULT 15;
