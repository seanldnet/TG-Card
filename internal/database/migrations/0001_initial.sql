CREATE TABLE IF NOT EXISTS admins (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	real_name TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'super',
	status INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
	id TEXT PRIMARY KEY,
	admin_id TEXT,
	action TEXT NOT NULL,
	resource TEXT NOT NULL DEFAULT '',
	ip_address TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	details TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_admin_created ON audit_logs(admin_id, created_at);
