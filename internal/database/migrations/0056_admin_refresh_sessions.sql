-- Store only hashed long-lived refresh tokens and support rotation/replay detection.
CREATE TABLE IF NOT EXISTS admin_refresh_sessions (
  token_hash TEXT PRIMARY KEY,
  admin_id TEXT NOT NULL,
  family_id TEXT NOT NULL,
  token_version INTEGER NOT NULL DEFAULT 0,
  expires_at BIGINT NOT NULL,
  revoked_at TEXT NOT NULL DEFAULT '',
  replaced_by_hash TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY(admin_id) REFERENCES admins(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_admin_refresh_sessions_admin ON admin_refresh_sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_refresh_sessions_family ON admin_refresh_sessions(family_id);
CREATE INDEX IF NOT EXISTS idx_admin_refresh_sessions_expiry ON admin_refresh_sessions(expires_at);
