-- Persist per-token admin sessions so logout can revoke only the current device/session.
CREATE TABLE admin_token_sessions (
  token_hash TEXT PRIMARY KEY,
  admin_id TEXT NOT NULL,
  expires_at BIGINT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(admin_id) REFERENCES admins(id) ON DELETE CASCADE
);
CREATE INDEX idx_admin_token_sessions_admin_id ON admin_token_sessions(admin_id);
