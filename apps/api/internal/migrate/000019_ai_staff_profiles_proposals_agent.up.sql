-- Versioned Markdown profiles per service account (AI staff).

CREATE TABLE service_account_profile_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    version INT NOT NULL,
    content_md TEXT NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (service_account_id, version)
);

CREATE INDEX idx_sa_profile_versions_sa ON service_account_profile_versions(service_account_id);

-- Pending/resolved edit proposals for space files (review before apply).

CREATE TYPE file_edit_proposal_status AS ENUM ('pending', 'accepted', 'rejected');

CREATE TABLE file_edit_proposals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    node_id UUID NOT NULL REFERENCES file_nodes(id) ON DELETE CASCADE,
    author_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    base_content_sha256 TEXT NOT NULL,
    proposed_content TEXT NOT NULL,
    status file_edit_proposal_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ NULL,
    resolved_by UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_file_edit_proposals_space_node ON file_edit_proposals(space_id, node_id);
CREATE INDEX idx_file_edit_proposals_org ON file_edit_proposals(organization_id);

-- Audit log for agent-tools / MCP harness invocations.

CREATE TABLE agent_tool_invocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NULL,
    tool TEXT NOT NULL,
    arguments_json JSONB NULL,
    result_json JSONB NULL,
    error_text TEXT NULL,
    duration_ms INT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_tool_invocations_org_created ON agent_tool_invocations(organization_id, created_at DESC);
