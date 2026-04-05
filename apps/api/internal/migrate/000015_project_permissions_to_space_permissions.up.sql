-- Rename project-scoped permissions to space-scoped permissions (breaking change).
-- This keeps existing roles working after the backend/frontend rename.

UPDATE role_permissions
SET permission = 'space.create'
WHERE permission = 'project.create';

UPDATE role_permissions
SET permission = 'space.delete'
WHERE permission = 'project.delete';

UPDATE role_permissions
SET permission = 'space.members.manage'
WHERE permission = 'project.members.manage';

