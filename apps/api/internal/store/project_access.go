package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListSpaceAllowedRoleIDs returns the allowlisted role IDs for a space.
// If empty, access is considered open to all org members (backward-compatible default).
func (s *Store) ListSpaceAllowedRoleIDs(ctx context.Context, spaceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT role_id
		FROM space_role_access
		WHERE space_id = $1
		ORDER BY role_id
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ReplaceSpaceAllowedRoles replaces the allowlist for a space.
func (s *Store) ReplaceSpaceAllowedRoles(ctx context.Context, spaceID uuid.UUID, roleIDs []uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM space_role_access WHERE space_id = $1`, spaceID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO space_role_access (space_id, role_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, spaceID, rid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UserCanAccessSpace returns whether the user can access a space by role allowlist.
// Admin override: org.manage or space.members.manage always grants access.
func (s *Store) UserCanAccessSpace(ctx context.Context, orgID, spaceID, userID uuid.UUID) (bool, error) {
	// Ensure legacy mapping exists so role-based checks work for old org_member.role values.
	_ = s.EnsureLegacyRoleMapped(ctx, orgID, userID)

	// Admin override.
	if ok, err := s.MemberHasPermission(ctx, orgID, userID, "org.manage"); err == nil && ok {
		return true, nil
	}
	if ok, err := s.MemberHasPermission(ctx, orgID, userID, "space.members.manage"); err == nil && ok {
		return true, nil
	}

	allowed, err := s.ListSpaceAllowedRoleIDs(ctx, spaceID)
	if err != nil {
		return false, err
	}
	// Default-open if no allowlist configured — but only for users who have workspace access
	// (board.read) or an explicit space_members row; members with no RBAC roles see nothing.
	if len(allowed) == 0 {
		if _, err := s.SpaceMemberRole(ctx, spaceID, userID); err == nil {
			return true, nil
		}
		if err != pgx.ErrNoRows {
			return false, err
		}
		ok, err := s.MemberHasPermission(ctx, orgID, userID, "board.read")
		if err != nil {
			return false, err
		}
		return ok, nil
	}
	userRoles, err := s.MemberRoleIDs(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	allowedSet := make(map[uuid.UUID]struct{}, len(allowed))
	for _, rid := range allowed {
		allowedSet[rid] = struct{}{}
	}
	for _, rid := range userRoles {
		if _, ok := allowedSet[rid]; ok {
			return true, nil
		}
	}
	return false, nil
}

// UserHasEffectiveSpaceAccess matches RequireSpaceMember: explicit space_members row
// (owner/member) grants access; otherwise org admin overrides and role allowlist via
// UserCanAccessSpace apply.
func (s *Store) UserHasEffectiveSpaceAccess(ctx context.Context, orgID, spaceID, userID uuid.UUID) (bool, error) {
	_, err := s.SpaceMemberRole(ctx, spaceID, userID)
	if err == nil {
		return true, nil
	}
	if err != pgx.ErrNoRows {
		return false, err
	}
	return s.UserCanAccessSpace(ctx, orgID, spaceID, userID)
}

// ListOrgMembersWithUserAccessibleToSpace returns org members who may access the space
// (same rules as RequireSpaceMember for this space).
func (s *Store) ListOrgMembersWithUserAccessibleToSpace(ctx context.Context, orgID, spaceID uuid.UUID) ([]OrgMemberWithUser, error) {
	all, err := s.ListOrgMembersWithUser(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]OrgMemberWithUser, 0, len(all))
	for _, m := range all {
		ok, err := s.UserHasEffectiveSpaceAccess(ctx, orgID, spaceID, m.UserID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, m)
		}
	}
	return out, nil
}

