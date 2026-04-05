-- Spaces (projects) are empty folders; multiple boards per space; chat rooms & file placeholders.

ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_project_id_key;

CREATE TABLE chat_rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chat_rooms_project ON chat_rooms(project_id);

CREATE TABLE space_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_space_files_project ON space_files(project_id);
