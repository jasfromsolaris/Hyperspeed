package rbac

import (
	"context"

	"github.com/google/uuid"

	"hyperspeed/api/internal/store"
)

// HasPermission checks whether a user has a permission in an organization.
// It also performs idempotent legacy mapping (admin/member -> system roles) to ease rollout.
func HasPermission(ctx context.Context, st *store.Store, orgID, userID uuid.UUID, perm Permission) (bool, error) {
	if err := st.EnsureSystemRoles(ctx, orgID); err != nil {
		return false, err
	}
	if err := st.EnsureLegacyRoleMapped(ctx, orgID, userID); err != nil {
		return false, err
	}
	return st.MemberHasPermission(ctx, orgID, userID, string(perm))
}

