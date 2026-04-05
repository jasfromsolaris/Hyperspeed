-- Tabular datasets per space (Parquet/CSV in object storage).

CREATE TYPE dataset_format AS ENUM ('parquet', 'csv');

CREATE TABLE space_datasets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    format dataset_format NOT NULL,
    storage_key TEXT NOT NULL,
    size_bytes BIGINT NULL,
    schema_json JSONB NULL,
    row_count_estimate BIGINT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (space_id, name)
);

CREATE INDEX idx_space_datasets_space ON space_datasets(space_id);
CREATE INDEX idx_space_datasets_org ON space_datasets(organization_id);

CREATE TABLE dataset_query_invocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    dataset_id UUID NOT NULL REFERENCES space_datasets(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    request_json JSONB NULL,
    row_count_returned INT NULL,
    duration_ms INT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dataset_query_invocations_ds ON dataset_query_invocations(dataset_id, created_at DESC);
