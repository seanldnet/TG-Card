ALTER TABLE payment_settings ADD COLUMN gmpay_base_url TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN gmpay_pid TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN gmpay_secret_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_settings ADD COLUMN gmpay_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_settings ADD COLUMN gmpay_notify_url TEXT NOT NULL DEFAULT '';
ALTER TABLE epusdt_payment_intents ADD COLUMN provider TEXT NOT NULL DEFAULT 'epusdt';
