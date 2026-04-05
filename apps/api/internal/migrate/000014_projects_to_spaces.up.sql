-- Rename backend terminology from projects -> spaces (breaking change).

-- Core table
ALTER TABLE IF EXISTS projects RENAME TO spaces;

-- Member + access allowlist join tables
ALTER TABLE IF EXISTS project_members RENAME TO space_members;
ALTER TABLE IF EXISTS project_role_access RENAME TO space_role_access;

-- Rename foreign key columns from project_id -> space_id
ALTER TABLE IF EXISTS boards RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS tasks RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS chat_rooms RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS chat_messages RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS space_files RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS file_nodes RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS space_members RENAME COLUMN project_id TO space_id;
ALTER TABLE IF EXISTS space_role_access RENAME COLUMN project_id TO space_id;

-- Rename common indexes (best-effort; ok if some don't exist depending on earlier migrations)
ALTER INDEX IF EXISTS idx_projects_org RENAME TO idx_spaces_org;

ALTER INDEX IF EXISTS idx_tasks_project RENAME TO idx_tasks_space;

ALTER INDEX IF EXISTS idx_chat_rooms_project RENAME TO idx_chat_rooms_space;
ALTER INDEX IF EXISTS idx_chat_messages_project_id RENAME TO idx_chat_messages_space_id;

ALTER INDEX IF EXISTS idx_space_files_project RENAME TO idx_space_files_space;

ALTER INDEX IF EXISTS idx_file_nodes_project_parent RENAME TO idx_file_nodes_space_parent;
ALTER INDEX IF EXISTS idx_file_nodes_project_deleted RENAME TO idx_file_nodes_space_deleted;
ALTER INDEX IF EXISTS idx_file_nodes_project_kind RENAME TO idx_file_nodes_space_kind;

ALTER INDEX IF EXISTS idx_project_members_user RENAME TO idx_space_members_user;

ALTER INDEX IF EXISTS idx_project_role_access_project RENAME TO idx_space_role_access_space;
ALTER INDEX IF EXISTS idx_project_role_access_role RENAME TO idx_space_role_access_role;

