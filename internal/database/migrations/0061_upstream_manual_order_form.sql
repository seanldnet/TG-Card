ALTER TABLE orders ADD COLUMN upstream_manual_form_data_ciphertext TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS upstream_manual_form_sessions (
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  telegram_chat_id TEXT NOT NULL,
  product_id TEXT NOT NULL,
  promotion_code TEXT NOT NULL DEFAULT '',
  schema_json TEXT NOT NULL,
  form_data_ciphertext TEXT NOT NULL,
  field_index INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(bot_id, telegram_chat_id)
);
CREATE INDEX IF NOT EXISTS idx_upstream_manual_form_sessions_expiry ON upstream_manual_form_sessions(expires_at);
