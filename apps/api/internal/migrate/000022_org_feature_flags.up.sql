-- Org-scoped feature flags for controlling optional product surfaces.

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS datasets_enabled BOOLEAN NOT NULL DEFAULT false;
