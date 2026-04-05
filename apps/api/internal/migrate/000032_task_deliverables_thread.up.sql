-- Task deliverable metadata, per-task message thread, and linked files under Deliverables folder.

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS deliverable_required BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS deliverable_instructions TEXT NOT NULL DEFAULT '';

CREATE TABLE task_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    content TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_task_messages_task_created ON task_messages(task_id, created_at);

CREATE TABLE task_deliverable_files (
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    file_node_id UUID NOT NULL REFERENCES file_nodes(id) ON DELETE CASCADE,
    added_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, file_node_id)
);

CREATE INDEX idx_task_deliverable_files_file ON task_deliverable_files(file_node_id);

-- Dedupe assignment notifications per (assignee, task, task version) after reassignment.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_notifications_task_assigned_user_task_version
  ON notifications (user_id, (payload->>'task_id'), (payload->>'task_version'))
  WHERE type = 'task.assigned';
