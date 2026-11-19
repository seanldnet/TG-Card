-- Bind durable Owner support sessions to the Telegram chat where reply mode was started.
-- Legacy sessions cannot be safely associated with a chat and must be reopened by the Owner.
ALTER TABLE support_owner_sessions ADD COLUMN chat_id TEXT NOT NULL DEFAULT '';
DELETE FROM support_owner_sessions WHERE chat_id = '';
