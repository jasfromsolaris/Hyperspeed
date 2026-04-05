package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const MaxServiceAccountProfileBytes = 256 * 1024

var ErrProfileTooLarge = errors.New("profile too large")

func insertDefaultServiceAccountProfileTx(ctx context.Context, tx pgx.Tx, serviceAccountID, createdBy uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO service_account_profile_versions (service_account_id, version, content_md, created_by)
		VALUES ($1, 1, $2, $3)
	`, serviceAccountID, DefaultServiceAccountProfileMarkdown(), createdBy)
	return err
}

// BackfillDefaultServiceAccountProfiles inserts version 1 for service accounts that have no profile rows yet.
func (s *Store) BackfillDefaultServiceAccountProfiles(ctx context.Context) error {
	md := DefaultServiceAccountProfileMarkdown()
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO service_account_profile_versions (service_account_id, version, content_md, created_by)
		SELECT sa.id, 1, $1, sa.created_by
		FROM service_accounts sa
		WHERE NOT EXISTS (
			SELECT 1 FROM service_account_profile_versions p WHERE p.service_account_id = sa.id
		)
	`, md)
	return err
}

// BackfillEmptyLatestServiceAccountProfiles appends a new version with the default Markdown when the
// latest saved profile is empty or whitespace-only (e.g. an accidental blank v1).
func (s *Store) BackfillEmptyLatestServiceAccountProfiles(ctx context.Context) error {
	md := DefaultServiceAccountProfileMarkdown()
	rows, err := s.Pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (service_account_id)
				service_account_id, content_md
			FROM service_account_profile_versions
			ORDER BY service_account_id, version DESC
		)
		SELECT sa.id, sa.organization_id, sa.created_by
		FROM service_accounts sa
		INNER JOIN latest l ON l.service_account_id = sa.id AND trim(l.content_md) = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var saID, orgID, createdBy uuid.UUID
		if err := rows.Scan(&saID, &orgID, &createdBy); err != nil {
			return err
		}
		if _, err := s.AppendServiceAccountProfile(ctx, orgID, saID, md, createdBy); err != nil {
			return err
		}
	}
	return rows.Err()
}

type ServiceAccountProfileVersion struct {
	ID               uuid.UUID `json:"id"`
	ServiceAccountID uuid.UUID `json:"service_account_id"`
	Version          int       `json:"version"`
	ContentMD        string    `json:"content_md"`
	CreatedBy        uuid.UUID `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s *Store) LatestServiceAccountProfile(ctx context.Context, serviceAccountID uuid.UUID) (ServiceAccountProfileVersion, error) {
	var v ServiceAccountProfileVersion
	err := s.Pool.QueryRow(ctx, `
		SELECT id, service_account_id, version, content_md, created_by, created_at
		FROM service_account_profile_versions
		WHERE service_account_id = $1
		ORDER BY version DESC
		LIMIT 1
	`, serviceAccountID).Scan(&v.ID, &v.ServiceAccountID, &v.Version, &v.ContentMD, &v.CreatedBy, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	return v, nil
}

func (s *Store) ListServiceAccountProfileVersions(ctx context.Context, serviceAccountID uuid.UUID, limit int) ([]ServiceAccountProfileVersion, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, service_account_id, version, content_md, created_by, created_at
		FROM service_account_profile_versions
		WHERE service_account_id = $1
		ORDER BY version DESC
		LIMIT $2
	`, serviceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceAccountProfileVersion
	for rows.Next() {
		var v ServiceAccountProfileVersion
		if err := rows.Scan(&v.ID, &v.ServiceAccountID, &v.Version, &v.ContentMD, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) AppendServiceAccountProfile(ctx context.Context, orgID, serviceAccountID uuid.UUID, contentMD string, createdBy uuid.UUID) (ServiceAccountProfileVersion, error) {
	if len(contentMD) > MaxServiceAccountProfileBytes {
		return ServiceAccountProfileVersion{}, ErrProfileTooLarge
	}
	var dummy int
	if err := s.Pool.QueryRow(ctx, `
		SELECT 1 FROM service_accounts WHERE id = $1 AND organization_id = $2
	`, serviceAccountID, orgID).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return ServiceAccountProfileVersion{}, pgx.ErrNoRows
		}
		return ServiceAccountProfileVersion{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var next int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1 FROM service_account_profile_versions WHERE service_account_id = $1
	`, serviceAccountID).Scan(&next)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}

	var v ServiceAccountProfileVersion
	err = tx.QueryRow(ctx, `
		INSERT INTO service_account_profile_versions (service_account_id, version, content_md, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, service_account_id, version, content_md, created_by, created_at
	`, serviceAccountID, next, contentMD, createdBy).Scan(
		&v.ID, &v.ServiceAccountID, &v.Version, &v.ContentMD, &v.CreatedBy, &v.CreatedAt,
	)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	return v, nil
}
