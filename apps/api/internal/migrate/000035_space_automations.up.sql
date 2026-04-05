-- Space-scoped automations (social, tunnels, etc.) with optional human approval.

CREATE TABLE space_automations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('social_post', 'reverse_tunnel', 'scheduled', 'webhook')),
  config JSONB NOT NULL DEFAULT '{}',
  status TEXT NOT NULL CHECK (status IN ('draft', 'pending_approval', 'active', 'paused', 'failed', 'rejected')),
  oauth_token_enc TEXT,
  created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_by_service_account_id UUID REFERENCES service_accounts(id) ON DELETE SET NULL,
  reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  reviewed_at TIMESTAMPTZ,
  rejection_reason TEXT,
  last_run_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_space_automations_space ON space_automations(space_id);
CREATE INDEX idx_space_automations_org ON space_automations(organization_id);
CREATE INDEX idx_space_automations_status ON space_automations(organization_id, space_id, status);

CREATE TABLE space_automation_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  automation_id UUID NOT NULL REFERENCES space_automations(id) ON DELETE CASCADE,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  success BOOLEAN NOT NULL DEFAULT false,
  error_message TEXT,
  external_ref TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_space_automation_runs_automation ON space_automation_runs(automation_id, started_at DESC);
