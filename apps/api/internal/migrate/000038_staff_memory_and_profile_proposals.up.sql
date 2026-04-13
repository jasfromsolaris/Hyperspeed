CREATE TYPE service_account_profile_proposal_status AS ENUM ('pending', 'accepted', 'rejected');

CREATE TABLE staff_memory_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    source_message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    reply_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ NULL,
    UNIQUE (service_account_id, source_message_id)
);

CREATE INDEX idx_staff_memory_runs_org_sa_created
    ON staff_memory_runs(organization_id, service_account_id, created_at DESC);

CREATE TABLE staff_memory_episodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    space_id UUID NULL REFERENCES spaces(id) ON DELETE SET NULL,
    chat_room_id UUID NULL REFERENCES chat_rooms(id) ON DELETE SET NULL,
    source_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    reply_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    summary TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    importance DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_staff_memory_episodes_org_sa_created
    ON staff_memory_episodes(organization_id, service_account_id, created_at DESC);

CREATE INDEX idx_staff_memory_episodes_search
    ON staff_memory_episodes
    USING GIN (to_tsvector('english', coalesce(summary, '') || ' ' || coalesce(details, '')));

CREATE TABLE staff_memory_facts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    episode_id UUID NULL REFERENCES staff_memory_episodes(id) ON DELETE SET NULL,
    source_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    statement TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ NULL,
    invalidated_at TIMESTAMPTZ NULL,
    supersedes_id UUID NULL REFERENCES staff_memory_facts(id) ON DELETE SET NULL
);

CREATE INDEX idx_staff_memory_facts_org_sa_created
    ON staff_memory_facts(organization_id, service_account_id, created_at DESC);

CREATE INDEX idx_staff_memory_facts_active
    ON staff_memory_facts(organization_id, service_account_id, created_at DESC)
    WHERE invalidated_at IS NULL;

CREATE INDEX idx_staff_memory_facts_search
    ON staff_memory_facts
    USING GIN (to_tsvector('english', coalesce(statement, '')));

CREATE TABLE staff_memory_procedures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    source_episode_id UUID NULL REFERENCES staff_memory_episodes(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    steps_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    success_count INT NOT NULL DEFAULT 0,
    failure_count INT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_staff_memory_procedures_org_sa_updated
    ON staff_memory_procedures(organization_id, service_account_id, updated_at DESC);

CREATE TABLE service_account_profile_proposals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_account_id UUID NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    source_message_id UUID NULL REFERENCES chat_messages(id) ON DELETE SET NULL,
    proposed_append_md TEXT NOT NULL,
    status service_account_profile_proposal_status NOT NULL DEFAULT 'pending',
    created_by UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ NULL,
    resolved_by UUID NULL REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_sa_profile_proposals_org_sa_status
    ON service_account_profile_proposals(organization_id, service_account_id, status, created_at DESC);
