ALTER TABLE admins ADD COLUMN email TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_admins_email_unique ON admins(email) WHERE email <> '';
CREATE TABLE IF NOT EXISTS admin_email_verification_codes (
  id TEXT PRIMARY KEY,
  admin_id TEXT NOT NULL,
  email TEXT NOT NULL,
  code_hash TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_admin_email_codes_admin ON admin_email_verification_codes(admin_id,created_at DESC);
