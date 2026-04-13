package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ServiceAccountProfileProposalStatus string

const (
	ServiceAccountProfileProposalPending  ServiceAccountProfileProposalStatus = "pending"
	ServiceAccountProfileProposalAccepted ServiceAccountProfileProposalStatus = "accepted"
	ServiceAccountProfileProposalRejected ServiceAccountProfileProposalStatus = "rejected"
)

var ErrProfileProposalNotPending = errors.New("profile proposal not pending")

type ServiceAccountProfileProposal struct {
	ID               uuid.UUID                         `json:"id"`
	OrganizationID   uuid.UUID                         `json:"organization_id"`
	ServiceAccountID uuid.UUID                         `json:"service_account_id"`
	SourceMessageID  *uuid.UUID                        `json:"source_message_id,omitempty"`
	ProposedAppendMD string                            `json:"proposed_append_md"`
	Status           ServiceAccountProfileProposalStatus `json:"status"`
	CreatedBy        *uuid.UUID                        `json:"created_by,omitempty"`
	CreatedAt        time.Time                         `json:"created_at"`
	ResolvedAt       *time.Time                        `json:"resolved_at,omitempty"`
	ResolvedBy       *uuid.UUID                        `json:"resolved_by,omitempty"`
}

func (s *Store) CreateServiceAccountProfileProposal(ctx context.Context, orgID, serviceAccountID uuid.UUID, sourceMessageID, createdBy *uuid.UUID, proposedAppendMD string) (ServiceAccountProfileProposal, error) {
	proposedAppendMD = strings.TrimSpace(proposedAppendMD)
	if proposedAppendMD == "" {
		return ServiceAccountProfileProposal{}, pgx.ErrNoRows
	}
	var out ServiceAccountProfileProposal
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO service_account_profile_proposals (
			organization_id, service_account_id, source_message_id, proposed_append_md, created_by
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, service_account_id, source_message_id, proposed_append_md, status::text,
			created_by, created_at, resolved_at, resolved_by
	`, orgID, serviceAccountID, sourceMessageID, proposedAppendMD, createdBy).Scan(
		&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.SourceMessageID, &out.ProposedAppendMD, &out.Status,
		&out.CreatedBy, &out.CreatedAt, &out.ResolvedAt, &out.ResolvedBy,
	)
	return out, err
}

func (s *Store) ListServiceAccountProfileProposals(ctx context.Context, orgID, serviceAccountID uuid.UUID, status ServiceAccountProfileProposalStatus, limit int) ([]ServiceAccountProfileProposal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	st := strings.TrimSpace(string(status))
	if st == "" {
		st = string(ServiceAccountProfileProposalPending)
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, source_message_id, proposed_append_md, status::text,
			created_by, created_at, resolved_at, resolved_by
		FROM service_account_profile_proposals
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND status::text = $3
		ORDER BY created_at DESC
		LIMIT $4
	`, orgID, serviceAccountID, st, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServiceAccountProfileProposal
	for rows.Next() {
		var row ServiceAccountProfileProposal
		if err := rows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.SourceMessageID, &row.ProposedAppendMD, &row.Status,
			&row.CreatedBy, &row.CreatedAt, &row.ResolvedAt, &row.ResolvedBy,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) GetServiceAccountProfileProposal(ctx context.Context, orgID, proposalID uuid.UUID) (ServiceAccountProfileProposal, error) {
	var out ServiceAccountProfileProposal
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, service_account_id, source_message_id, proposed_append_md, status::text,
			created_by, created_at, resolved_at, resolved_by
		FROM service_account_profile_proposals
		WHERE organization_id = $1 AND id = $2
	`, orgID, proposalID).Scan(
		&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.SourceMessageID, &out.ProposedAppendMD, &out.Status,
		&out.CreatedBy, &out.CreatedAt, &out.ResolvedAt, &out.ResolvedBy,
	)
	return out, err
}

func (s *Store) ResolveServiceAccountProfileProposalRejected(ctx context.Context, orgID, proposalID, resolverID uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE service_account_profile_proposals
		SET status = 'rejected', resolved_at = now(), resolved_by = $3
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
	`, proposalID, orgID, resolverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrProfileProposalNotPending
	}
	return nil
}

func (s *Store) AcceptServiceAccountProfileProposal(ctx context.Context, orgID, proposalID, resolverID uuid.UUID) (ServiceAccountProfileVersion, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var proposal ServiceAccountProfileProposal
	err = tx.QueryRow(ctx, `
		SELECT id, organization_id, service_account_id, source_message_id, proposed_append_md, status::text,
			created_by, created_at, resolved_at, resolved_by
		FROM service_account_profile_proposals
		WHERE id = $1 AND organization_id = $2
		FOR UPDATE
	`, proposalID, orgID).Scan(
		&proposal.ID, &proposal.OrganizationID, &proposal.ServiceAccountID, &proposal.SourceMessageID, &proposal.ProposedAppendMD, &proposal.Status,
		&proposal.CreatedBy, &proposal.CreatedAt, &proposal.ResolvedAt, &proposal.ResolvedBy,
	)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	if proposal.Status != ServiceAccountProfileProposalPending {
		return ServiceAccountProfileVersion{}, ErrProfileProposalNotPending
	}

	latest := ""
	_ = tx.QueryRow(ctx, `
		SELECT content_md
		FROM service_account_profile_versions
		WHERE service_account_id = $1
		ORDER BY version DESC
		LIMIT 1
	`, proposal.ServiceAccountID).Scan(&latest)
	nextContent := strings.TrimSpace(latest)
	appendBlock := strings.TrimSpace(proposal.ProposedAppendMD)
	if nextContent == "" {
		nextContent = appendBlock
	} else {
		nextContent = nextContent + "\n\n" + appendBlock
	}
	if len(nextContent) > MaxServiceAccountProfileBytes {
		return ServiceAccountProfileVersion{}, ErrProfileTooLarge
	}
	var nextVersion int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) + 1
		FROM service_account_profile_versions
		WHERE service_account_id = $1
	`, proposal.ServiceAccountID).Scan(&nextVersion); err != nil {
		return ServiceAccountProfileVersion{}, err
	}

	var out ServiceAccountProfileVersion
	err = tx.QueryRow(ctx, `
		INSERT INTO service_account_profile_versions (service_account_id, version, content_md, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, service_account_id, version, content_md, created_by, created_at
	`, proposal.ServiceAccountID, nextVersion, nextContent, resolverID).Scan(
		&out.ID, &out.ServiceAccountID, &out.Version, &out.ContentMD, &out.CreatedBy, &out.CreatedAt,
	)
	if err != nil {
		return ServiceAccountProfileVersion{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE service_account_profile_proposals
		SET status = 'accepted', resolved_at = now(), resolved_by = $3
		WHERE id = $1 AND organization_id = $2
	`, proposalID, orgID, resolverID); err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ServiceAccountProfileVersion{}, err
	}
	return out, nil
}

