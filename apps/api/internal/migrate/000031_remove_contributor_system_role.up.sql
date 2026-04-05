-- Drop the Contributor system role; only Owner remains as the built-in system role.
-- New members and service accounts start with no RBAC roles until an owner assigns them.

DELETE FROM member_roles
WHERE role_id IN (SELECT id FROM roles WHERE name = 'Contributor');

DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE name = 'Contributor');

DELETE FROM roles WHERE name = 'Contributor' AND is_system = true;
