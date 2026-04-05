-- Per-space role allowlist (org-scoped roles applied to projects/spaces).

CREATE TABLE IF NOT EXISTS project_role_access (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_project_role_access_project ON project_role_access(project_id);
CREATE INDEX IF NOT EXISTS idx_project_role_access_role ON project_role_access(role_id);

