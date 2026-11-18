-- Persist the ten-minute guard for automatic follow-up ticket creation.
CREATE TABLE IF NOT EXISTS support_followup_throttles (
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  last_ticket_at TEXT NOT NULL,
  PRIMARY KEY(telegram_user_id)
);

-- User confirmation is idempotent even when Telegram redelivers a callback update.
CREATE UNIQUE INDEX IF NOT EXISTS idx_support_ticket_audits_action_actor
  ON support_ticket_audits(ticket_id, action, actor_id)
  WHERE action = 'user_confirmed_resolved';
