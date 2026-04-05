-- Grant datasets.read / datasets.write alongside existing file permissions so non-Owner
-- roles (e.g. custom roles cloned from file access) keep parity without manual Org → Roles edits.

INSERT INTO role_permissions (role_id, permission)
SELECT DISTINCT rp.role_id, 'datasets.read'
FROM role_permissions rp
WHERE rp.permission = 'files.read'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions x
    WHERE x.role_id = rp.role_id AND x.permission = 'datasets.read'
  );

INSERT INTO role_permissions (role_id, permission)
SELECT DISTINCT rp.role_id, 'datasets.write'
FROM role_permissions rp
WHERE rp.permission = 'files.write'
  AND NOT EXISTS (
    SELECT 1 FROM role_permissions x
    WHERE x.role_id = rp.role_id AND x.permission = 'datasets.write'
  );
