-- Dual-provider AI staff: OpenRouter vs Cursor; org OpenRouter key; optional chat message metadata for agent runs.

-- Per–service-account backend and defaults (VISION: roster with provider + model or default repo).
ALTER TABLE service_accounts
    ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'openrouter',
    ADD COLUMN IF NOT EXISTS openrouter_model TEXT,
    ADD COLUMN IF NOT EXISTS cursor_default_repo_url TEXT,
    ADD COLUMN IF NOT EXISTS cursor_default_ref TEXT;

ALTER TABLE service_accounts DROP CONSTRAINT IF EXISTS service_accounts_provider_check;
ALTER TABLE service_accounts ADD CONSTRAINT service_accounts_provider_check
    CHECK (provider IN ('openrouter', 'cursor'));

-- Org-scoped OpenRouter API key (same pattern as Cursor key).
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS openrouter_api_key_enc TEXT,
    ADD COLUMN IF NOT EXISTS openrouter_api_key_hint TEXT,
    ADD COLUMN IF NOT EXISTS openrouter_api_key_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS openrouter_api_key_updated_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- Optional JSON for agent-run cards (e.g. cursor cloud agent id + link).
ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS metadata JSONB;

-- Existing AI staff rows default to OpenRouter with a concrete model.
UPDATE service_accounts
SET openrouter_model = 'openai/gpt-4o-mini'
WHERE provider = 'openrouter' AND (openrouter_model IS NULL OR trim(openrouter_model) = '');
