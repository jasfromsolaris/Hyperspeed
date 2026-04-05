-- Notification idempotency helpers for chat mentions.
-- Prevent duplicate mention notifications for the same (user, message).

CREATE UNIQUE INDEX IF NOT EXISTS uniq_notifications_chat_mention_user_message
  ON notifications (user_id, (payload->>'message_id'))
  WHERE type = 'chat.mention';

