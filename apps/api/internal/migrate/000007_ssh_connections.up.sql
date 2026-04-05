-- User-owned SSH connection profiles (encrypted secrets).

CREATE TABLE IF NOT EXISTS ssh_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    port INT NOT NULL DEFAULT 22,
    username TEXT NOT NULL,
    private_key_enc TEXT NOT NULL,
    passphrase_enc TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ssh_connections_org_owner ON ssh_connections(organization_id, owner_user_id);

