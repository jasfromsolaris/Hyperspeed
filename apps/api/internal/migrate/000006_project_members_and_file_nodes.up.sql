-- Per-space membership and Google Drive-like file metadata.

CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user ON project_members(user_id);

-- file_nodes is a hierarchical tree of folders/files. Actual bytes live in object storage.
CREATE TABLE file_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id UUID NULL REFERENCES file_nodes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('folder', 'file')),
    name TEXT NOT NULL,
    mime_type TEXT NULL,
    size_bytes BIGINT NULL,
    storage_key TEXT NULL,
    checksum_sha256 TEXT NULL,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_file_nodes_project_parent ON file_nodes(project_id, parent_id);
CREATE INDEX idx_file_nodes_project_deleted ON file_nodes(project_id, deleted_at);
CREATE INDEX idx_file_nodes_project_kind ON file_nodes(project_id, kind);

-- Basic name search.
CREATE INDEX idx_file_nodes_name ON file_nodes(name);

