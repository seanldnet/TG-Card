-- Durable Owner support sessions, Telegram update idempotency and owner audit trail.
ALTER TABLE support_ticket_messages ADD COLUMN telegram_update_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_support_ticket_messages_update ON support_ticket_messages(ticket_id, sender_type, sender_id, telegram_update_id) WHERE telegram_update_id <> '';
CREATE TABLE IF NOT EXISTS support_owner_sessions (
  bot_id TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  ticket_id TEXT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
  phase TEXT NOT NULL DEFAULT 'reply',
  expires_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(bot_id, owner_id)
);
CREATE INDEX IF NOT EXISTS idx_support_owner_sessions_expiry ON support_owner_sessions(expires_at);
CREATE TABLE IF NOT EXISTS support_ticket_audits (
  id TEXT PRIMARY KEY,
  ticket_id TEXT NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  actor_id TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_support_ticket_audits_ticket_created ON support_ticket_audits(ticket_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_support_ticket_messages_attachment_dedupe ON support_ticket_messages(ticket_id, sender_type, sender_id, attachment_file_id) WHERE attachment_file_id <> '';
