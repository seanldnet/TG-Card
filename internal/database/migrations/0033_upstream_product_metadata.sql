ALTER TABLE product_categories ADD COLUMN upstream_icon TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN upstream_sku_code TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN upstream_spec_values_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE products ADD COLUMN upstream_fulfillment_type TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN upstream_manual_form_schema_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE products ADD COLUMN upstream_tags_json TEXT NOT NULL DEFAULT '[]';
