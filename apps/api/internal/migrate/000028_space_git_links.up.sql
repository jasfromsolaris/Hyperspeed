-- One Git remote link per space (HTTPS + encrypted PAT). See docs/adr/ide-git-github.md

CREATE TABLE space_git_links (
    space_id UUID PRIMARY KEY REFERENCES spaces(id) ON DELETE CASCADE,
    remote_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    root_folder_id UUID NULL REFERENCES file_nodes(id) ON DELETE SET NULL,
    token_ciphertext TEXT NULL,
    token_last4 TEXT NULL,
    last_commit_sha TEXT NULL,
    last_error TEXT NULL,
    last_sync_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_space_git_links_root_folder ON space_git_links(root_folder_id);
