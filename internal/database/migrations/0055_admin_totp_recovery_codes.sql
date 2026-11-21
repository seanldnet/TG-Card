ALTER TABLE admins ADD COLUMN totp_recovery_code_hashes TEXT NOT NULL DEFAULT '[]';
