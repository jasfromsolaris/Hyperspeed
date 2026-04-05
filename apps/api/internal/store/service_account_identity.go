package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ServiceAccountIdentity is set when the user is a service-account principal.
type ServiceAccountIdentity struct {
	ServiceAccountID uuid.UUID `json:"id"`
	OrganizationID   uuid.UUID `json:"organization_id"`
}

// ServiceAccountIdentityByUser returns identity if this user backs a service account (at most one globally).
func (s *Store) ServiceAccountIdentityByUser(ctx context.Context, userID uuid.UUID) (*ServiceAccountIdentity, error) {
	var out ServiceAccountIdentity
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id FROM service_accounts WHERE user_id = $1
	`, userID).Scan(&out.ServiceAccountID, &out.OrganizationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// ServiceAccountInOrg reports whether the user is a service account for this organization.
func (s *Store) ServiceAccountInOrg(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		SELECT 1 FROM service_accounts WHERE organization_id = $1 AND user_id = $2
	`, orgID, userID).Scan(&n)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FilterServiceAccountUserIDsInOrg returns the subset of user IDs that are service accounts in org.
func (s *Store) FilterServiceAccountUserIDsInOrg(ctx context.Context, orgID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(userIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT user_id
		FROM service_accounts
		WHERE organization_id = $1
		  AND user_id = ANY($2)
	`, orgID, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0, len(userIDs))
	for rows.Next() {
		var uid uuid.UUID
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}
