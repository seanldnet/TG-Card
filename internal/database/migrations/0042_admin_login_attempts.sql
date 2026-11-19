-- Persist administrator login failures across process restarts and replicas.
CREATE TABLE admin_login_attempts (
  id TEXT PRIMARY KEY,
  ip_hash TEXT NOT NULL,
  attempted_at BIGINT NOT NULL
);
CREATE INDEX idx_admin_login_attempts_ip_time ON admin_login_attempts(ip_hash, attempted_at);
