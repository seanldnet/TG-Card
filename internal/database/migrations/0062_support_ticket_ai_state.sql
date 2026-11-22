-- Keep AI preprocessing state separate from the support ticket's business lifecycle.
CREATE TABLE IF NOT EXISTS support_ticket_ai_states (
  ticket_id TEXT PRIMARY KEY REFERENCES support_tickets(id) ON DELETE CASCADE,
  ai_status TEXT NOT NULL DEFAULT 'not_used',
  ai_reason TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_ai_states_status
  ON support_ticket_ai_states(ai_status, updated_at);
