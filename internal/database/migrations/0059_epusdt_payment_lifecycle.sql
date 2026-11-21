-- Epusdt payment lifecycle migration
ALTER TABLE payment_settings ADD COLUMN epusdt_notify_url TEXT NOT NULL DEFAULT '';
