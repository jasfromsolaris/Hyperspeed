-- Server-backed IDE preview sessions (Phase 2 stub: static snapshot + token-gated HTTP).

CREATE TABLE preview_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'failed', 'expired')),
    command TEXT,
    cwd TEXT,
    access_token TEXT NOT NULL,
    error_message TEXT,
    snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_preview_sessions_space ON preview_sessions(space_id);
CREATE INDEX idx_preview_sessions_expires ON preview_sessions(expires_at);
