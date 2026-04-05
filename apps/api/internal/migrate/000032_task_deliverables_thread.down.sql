DROP INDEX IF EXISTS uniq_notifications_task_assigned_user_task_version;

DROP TABLE IF EXISTS task_deliverable_files;
DROP TABLE IF EXISTS task_messages;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS deliverable_instructions,
    DROP COLUMN IF EXISTS deliverable_required;
