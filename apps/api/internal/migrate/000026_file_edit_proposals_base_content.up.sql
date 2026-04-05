-- Snapshot of file text at proposal creation so accepted/rejected proposals still show a meaningful diff.

ALTER TABLE file_edit_proposals
    ADD COLUMN IF NOT EXISTS base_content TEXT;

COMMENT ON COLUMN file_edit_proposals.base_content IS 'File text when the proposal was created; preserved for diff UI after accept.';
