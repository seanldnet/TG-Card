-- P0-2: canonical global exception store. Keep finance_exceptions as a
-- compatibility source table; existing records are copied without losing IDs.
CREATE TABLE IF NOT EXISTS system_exceptions (
  id TEXT PRIMARY KEY,
  source_key TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'warning',
  entity_type TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  title TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  resolution_note TEXT NOT NULL DEFAULT '',
  resolved_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT NOT NULL DEFAULT '',
  occurrence_count INTEGER NOT NULL DEFAULT 1,
  last_seen_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_system_exceptions_status ON system_exceptions(status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_system_exceptions_category ON system_exceptions(category,status,updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_system_exceptions_last_seen ON system_exceptions(last_seen_at DESC);

INSERT INTO system_exceptions (
  id,source_key,category,severity,entity_type,entity_id,title,detail,status,
  resolution_note,resolved_by,created_at,updated_at,resolved_at,occurrence_count,last_seen_at
)
SELECT id,source_key,category,severity,entity_type,entity_id,title,detail,status,
       resolution_note,resolved_by,created_at,updated_at,resolved_at,1,updated_at
FROM finance_exceptions WHERE 1=1
ON CONFLICT(source_key) DO NOTHING;
