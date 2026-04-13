package rbac

import (
	"context"

	"github.com/google/uuid"

	"hyperspeed/api/internal/store"
)

// HasPermission checks whether a user has a permission in an organization.
// It also performs idempotent legacy mapping (admin/member -> system roles) to ease rollout.
//
// If RBAC rows are missing or inconsistent on a long-lived deployment, organization_members.role
// may still be the legacy enum value "admin". Those users are equivalent to the built-in Owner
// role for authorization (same permission set as AllPermissions).
func HasPermission(ctx context.Context, st *store.Store, orgID, userID uuid.UUID, perm Permission) (bool, error) {
	if err := st.EnsureSystemRoles(ctx, orgID); err != nil {
		return false, err
	}
	if err := st.EnsureLegacyRoleMapped(ctx, orgID, userID); err != nil {
		return false, err
	}
	ok, err := st.MemberHasPermission(ctx, orgID, userID, string(perm))
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	legacy, err := st.IsLegacyOrgAdmin(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	if !legacy {
		return false, nil
	}
	for _, p := range AllPermissions {
		if p == perm {
			return true, nil
		}
	}
	return false, nil
}

