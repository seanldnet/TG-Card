-- Durable Telegram broadcast jobs and per-recipient delivery state.
CREATE TABLE IF NOT EXISTS broadcast_tasks (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  message_text TEXT NOT NULL,
  audience_type TEXT NOT NULL DEFAULT 'all',
  status TEXT NOT NULL DEFAULT 'queued',
  scheduled_at TEXT NOT NULL,
  started_at TEXT NOT NULL DEFAULT '',
  completed_at TEXT NOT NULL DEFAULT '',
  total_count INTEGER NOT NULL DEFAULT 0,
  sent_count INTEGER NOT NULL DEFAULT 0,
  failed_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_broadcast_tasks_due ON broadcast_tasks(status, scheduled_at);
CREATE TABLE IF NOT EXISTS broadcast_recipients (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES broadcast_tasks(id) ON DELETE CASCADE,
  telegram_user_id TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  sent_at TEXT NOT NULL DEFAULT '',
  UNIQUE(task_id, telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_broadcast_recipients_pending ON broadcast_recipients(task_id, status);
CREATE TABLE IF NOT EXISTS broadcast_opt_outs (
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY(bot_id, telegram_user_id)
);

-- Track every Bot/user relationship rather than only the user's most recently used Bot.
CREATE TABLE IF NOT EXISTS telegram_user_bots (
  bot_id TEXT NOT NULL,
  telegram_user_id TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  PRIMARY KEY(bot_id, telegram_user_id)
);
CREATE INDEX IF NOT EXISTS idx_telegram_user_bots_recent ON telegram_user_bots(bot_id, last_seen_at);

INSERT INTO telegram_user_bots(bot_id,telegram_user_id,chat_id,last_seen_at)
SELECT last_seen_bot_id,telegram_user_id,telegram_user_id,updated_at FROM telegram_users
WHERE last_seen_bot_id IS NOT NULL AND last_seen_bot_id<>''
ON CONFLICT(bot_id,telegram_user_id) DO NOTHING;
