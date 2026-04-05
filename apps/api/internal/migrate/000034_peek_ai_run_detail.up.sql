ALTER TABLE chat_ai_mention_replies
  ADD COLUMN IF NOT EXISTS run_detail JSONB NULL;

COMMENT ON COLUMN chat_ai_mention_replies.run_detail IS 'Structured Peek log: reasoning, tool calls, file ops (OpenRouter trace, Cursor conversation, etc.)';
