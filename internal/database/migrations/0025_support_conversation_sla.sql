-- Extend after-sales tickets with ownership, SLA tracking and a durable conversation timeline.
ALTER TABLE support_tickets ADD COLUMN assigned_to TEXT NOT NULL DEFAULT '';
ALTER TABLE support_tickets ADD COLUMN sla_due_at TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS support_ticket_messages (
  id TEXT PRIMARY KEY,
  ticket_id TEXT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
  sender_type TEXT NOT NULL CHECK (sender_type IN ('user','admin','system')),
  sender_id TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  attachment_file_id TEXT NOT NULL DEFAULT '',
  attachment_name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_messages_ticket_created ON support_ticket_messages(ticket_id, created_at);
CREATE INDEX IF NOT EXISTS idx_support_tickets_assigned_sla ON support_tickets(assigned_to, sla_due_at, status);
