-- Org-scoped Cursor API key (ciphertext + non-secret hint for UI).

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS cursor_api_key_enc TEXT,
    ADD COLUMN IF NOT EXISTS cursor_api_key_hint TEXT,
    ADD COLUMN IF NOT EXISTS cursor_api_key_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cursor_api_key_updated_by UUID REFERENCES users(id) ON DELETE SET NULL;
