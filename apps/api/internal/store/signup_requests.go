package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SignupRequestStatus is a staff open-signup approval row.
type SignupRequestStatus string

const (
	SignupPending  SignupRequestStatus = "pending"
	SignupApproved SignupRequestStatus = "approved"
	SignupDenied   SignupRequestStatus = "denied"
)

type SignupRequest struct {
	ID                uuid.UUID           `json:"id"`
	OrganizationID    uuid.UUID           `json:"organization_id"`
	UserID            uuid.UUID           `json:"user_id"`
	Status            SignupRequestStatus `json:"status"`
	CreatedAt         time.Time           `json:"created_at"`
	ResolvedAt        *time.Time          `json:"resolved_at,omitempty"`
	ResolvedByUserID  *uuid.UUID          `json:"resolved_by_user_id,omitempty"`
	Email             string              `json:"email"`
	DisplayName       *string             `json:"display_name,omitempty"`
}

// FirstOrganization returns the only organization row (singleton instance), if any.
func (s *Store) FirstOrganization(ctx context.Context) (Organization, error) {
	var o Organization
	var ipu, gss, poo sql.NullString
	err := s.Pool.QueryRow(ctx, `
		SELECT id, name, slug, datasets_enabled, open_signups_enabled, created_at,
			intended_public_url, gifted_subdomain_slug, public_origin_override
		FROM organizations
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&o.ID, &o.Name, &o.Slug, &o.DatasetsEnabled, &o.OpenSignupsEnabled, &o.CreatedAt, &ipu, &gss, &poo)
	if err != nil {
		return o, err
	}
	o.IntendedPublicURL = nullStrPtr(ipu)
	o.GiftedSubdomainSlug = nullStrPtr(gss)
	o.PublicOriginOverride = nullStrPtr(poo)
	return o, nil
}

// CreateSignupRequest inserts a pending signup request.
func (s *Store) CreateSignupRequest(ctx context.Context, orgID, userID uuid.UUID) (SignupRequest, error) {
	var r SignupRequest
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO signup_requests (organization_id, user_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, organization_id, user_id, status, created_at, resolved_at, resolved_by_user_id
	`, orgID, userID).Scan(
		&r.ID, &r.OrganizationID, &r.UserID, &r.Status, &r.CreatedAt, &r.ResolvedAt, &r.ResolvedByUserID,
	)
	return r, err
}

// PendingSignupRequestForUser returns the pending request for this user, if any.
func (s *Store) PendingSignupRequestForUser(ctx context.Context, userID uuid.UUID) (*SignupRequest, error) {
	var r SignupRequest
	err := s.Pool.QueryRow(ctx, `
		SELECT sr.id, sr.organization_id, sr.user_id, sr.status, sr.created_at, sr.resolved_at, sr.resolved_by_user_id,
		       u.email, u.display_name
		FROM signup_requests sr
		JOIN users u ON u.id = sr.user_id
		WHERE sr.user_id = $1 AND sr.status = 'pending'
		ORDER BY sr.created_at DESC
		LIMIT 1
	`, userID).Scan(
		&r.ID, &r.OrganizationID, &r.UserID, &r.Status, &r.CreatedAt, &r.ResolvedAt, &r.ResolvedByUserID,
		&r.Email, &r.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListSignupRequests lists requests for an org (pending first).
func (s *Store) ListSignupRequests(ctx context.Context, orgID uuid.UUID) ([]SignupRequest, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT sr.id, sr.organization_id, sr.user_id, sr.status, sr.created_at, sr.resolved_at, sr.resolved_by_user_id,
		       u.email, u.display_name
		FROM signup_requests sr
		JOIN users u ON u.id = sr.user_id
		WHERE sr.organization_id = $1 AND sr.status = 'pending'
		ORDER BY sr.created_at ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SignupRequest
	for rows.Next() {
		var r SignupRequest
		if err := rows.Scan(
			&r.ID, &r.OrganizationID, &r.UserID, &r.Status, &r.CreatedAt, &r.ResolvedAt, &r.ResolvedByUserID,
			&r.Email, &r.DisplayName,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResolveSignupRequest sets status to approved or denied and adds membership when approved.
// Approved users join with legacy member row only; RBAC roles are assigned by an owner in org settings.
func (s *Store) ResolveSignupRequest(ctx context.Context, orgID, requestID, resolverUserID uuid.UUID, approve bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var targetUser uuid.UUID
	var status SignupRequestStatus
	if approve {
		status = SignupApproved
	} else {
		status = SignupDenied
	}
	err = tx.QueryRow(ctx, `
		UPDATE signup_requests
		SET status = $4, resolved_at = now(), resolved_by_user_id = $3
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
		RETURNING user_id
	`, requestID, orgID, resolverUserID, status).Scan(&targetUser)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return err
	}

	if approve {
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, role)
			VALUES ($1, $2, $3)
			ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role
		`, orgID, targetUser, RoleMember); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if !approve {
		return nil
	}
	return s.EnsureSystemRoles(ctx, orgID)
}
