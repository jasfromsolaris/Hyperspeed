-- Remove legacy default system roles. New orgs should only have "Owner".
-- Safe/idempotent: if roles don't exist, no-op.

DELETE FROM roles
WHERE is_system = true
  AND name IN ('Admin', 'Member');

