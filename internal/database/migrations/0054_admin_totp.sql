-- Store TOTP secrets encrypted by the application secret manager.
ALTER TABLE admins ADD COLUMN totp_secret_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN totp_pending_secret_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE admins ADD COLUMN totp_enabled_at TEXT NOT NULL DEFAULT '';
ALTER TABLE admins ADD COLUMN totp_recovery_codes_hashes TEXT NOT NULL DEFAULT '[]';
ALTER TABLE admins ADD COLUMN totp_pending_recovery_codes_hashes TEXT NOT NULL DEFAULT '[]';
