-- One-time, short-lived proof that a sensitive administrator action was re-authenticated.
CREATE TABLE IF NOT EXISTS admin_action_confirmations (
  token_hash TEXT PRIMARY KEY,
  admin_id TEXT NOT NULL,
  action_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  expires_at BIGINT NOT NULL,
  consumed_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY(admin_id) REFERENCES admins(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_admin_action_confirmations_admin ON admin_action_confirmations(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_action_confirmations_expiry ON admin_action_confirmations(expires_at);
