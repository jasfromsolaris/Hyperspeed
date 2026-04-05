-- Org-level open signups and staff signup approval queue.

ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS open_signups_enabled BOOLEAN NOT NULL DEFAULT true;

CREATE TABLE IF NOT EXISTS signup_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'denied')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ NULL,
  resolved_by_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_signup_requests_org ON signup_requests(organization_id);
CREATE INDEX IF NOT EXISTS idx_signup_requests_org_status ON signup_requests(organization_id, status);

-- At most one pending request per (org, user).
CREATE UNIQUE INDEX IF NOT EXISTS signup_requests_one_pending_per_org_user
  ON signup_requests(organization_id, user_id)
  WHERE status = 'pending';
