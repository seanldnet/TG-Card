CREATE TABLE IF NOT EXISTS support_tickets (
  id TEXT PRIMARY KEY,
  ticket_no TEXT NOT NULL UNIQUE,
  order_id TEXT NOT NULL,
  order_no TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  telegram_chat_id TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT 'other',
  description TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  admin_reply TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  resolved_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_support_tickets_status_created ON support_tickets(status, created_at);
CREATE INDEX IF NOT EXISTS idx_support_tickets_user_bot_created ON support_tickets(telegram_user_id, bot_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_support_tickets_active_order ON support_tickets(order_id) WHERE status IN ('open','in_progress','waiting_user');
