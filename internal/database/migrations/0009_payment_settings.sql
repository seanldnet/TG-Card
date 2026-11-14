CREATE TABLE IF NOT EXISTS payment_settings (
  id INTEGER PRIMARY KEY,
  tron_api_key_ciphertext TEXT NOT NULL DEFAULT '',
  tron_receive_address TEXT NOT NULL DEFAULT '',
  tron_usdt_contract TEXT NOT NULL DEFAULT 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t',
  tron_enabled INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);
