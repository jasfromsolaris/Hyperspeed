-- Rename the non-owner system role from "Member" to "Contributor".
-- Keeps the same role row id and member_roles assignments; EnsureSystemRoles uses "Contributor" going forward.

UPDATE roles
SET name = 'Contributor', updated_at = now()
WHERE is_system = true AND name = 'Member';
