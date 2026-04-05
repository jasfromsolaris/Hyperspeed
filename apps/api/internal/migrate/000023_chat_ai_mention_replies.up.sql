CREATE TABLE IF NOT EXISTS chat_ai_mention_replies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    chat_room_id UUID NOT NULL REFERENCES chat_rooms(id) ON DELETE CASCADE,
    source_message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    ai_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requested_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ NULL,
    UNIQUE (source_message_id, ai_user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_ai_mention_replies_org ON chat_ai_mention_replies(organization_id, created_at DESC);
