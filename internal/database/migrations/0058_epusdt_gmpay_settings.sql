ALTER TABLE payment_settings ADD COLUMN epusdt_base_url TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN epusdt_pid TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN epusdt_secret_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN epusdt_enabled INTEGER NOT NULL DEFAULT 0;
