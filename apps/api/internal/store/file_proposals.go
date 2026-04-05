package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type FileEditProposalStatus string

const (
	ProposalPending  FileEditProposalStatus = "pending"
	ProposalAccepted FileEditProposalStatus = "accepted"
	ProposalRejected FileEditProposalStatus = "rejected"
)

type FileEditProposal struct {
	ID                 uuid.UUID              `json:"id"`
	OrganizationID     uuid.UUID              `json:"organization_id"`
	SpaceID            uuid.UUID              `json:"space_id"`
	NodeID             uuid.UUID              `json:"node_id"`
	AuthorUserID       uuid.UUID              `json:"author_user_id"`
	BaseContentSHA256  string                 `json:"base_content_sha256"`
	BaseContent        *string                `json:"base_content,omitempty"`
	ProposedContent    string                 `json:"proposed_content"`
	Status             FileEditProposalStatus `json:"status"`
	CreatedAt          time.Time              `json:"created_at"`
	ResolvedAt         *time.Time             `json:"resolved_at,omitempty"`
	ResolvedBy         *uuid.UUID             `json:"resolved_by,omitempty"`
}

const MaxProposalContentBytes = 2 << 20 // match GetText cap

var (
	ErrProposalStale           = errors.New("base file changed")
	ErrProposalNotPending      = errors.New("proposal not pending")
	ErrProposalContentTooLarge = errors.New("proposed content too large")
)

func (s *Store) CreateFileEditProposal(ctx context.Context, orgID, spaceID, nodeID, authorID uuid.UUID, baseSHA, baseContent, proposed string) (FileEditProposal, error) {
	if len(proposed) > MaxProposalContentBytes || len(baseContent) > MaxProposalContentBytes {
		return FileEditProposal{}, ErrProposalContentTooLarge
	}
	var p FileEditProposal
	var bc sql.NullString
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO file_edit_proposals (
			organization_id, space_id, node_id, author_user_id,
			base_content_sha256, base_content, proposed_content, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		RETURNING id, organization_id, space_id, node_id, author_user_id,
			base_content_sha256, base_content, proposed_content, status::text, created_at, resolved_at, resolved_by
	`, orgID, spaceID, nodeID, authorID, baseSHA, baseContent, proposed).Scan(
		&p.ID, &p.OrganizationID, &p.SpaceID, &p.NodeID, &p.AuthorUserID,
		&p.BaseContentSHA256, &bc, &p.ProposedContent, &p.Status, &p.CreatedAt, &p.ResolvedAt, &p.ResolvedBy,
	)
	if err != nil {
		return FileEditProposal{}, err
	}
	if bc.Valid {
		s := bc.String
		p.BaseContent = &s
	}
	return p, nil
}

func (s *Store) GetFileEditProposal(ctx context.Context, orgID, proposalID uuid.UUID) (FileEditProposal, error) {
	var p FileEditProposal
	var bc sql.NullString
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, space_id, node_id, author_user_id,
			base_content_sha256, base_content, proposed_content, status::text, created_at, resolved_at, resolved_by
		FROM file_edit_proposals
		WHERE id = $1 AND organization_id = $2
	`, proposalID, orgID).Scan(
		&p.ID, &p.OrganizationID, &p.SpaceID, &p.NodeID, &p.AuthorUserID,
		&p.BaseContentSHA256, &bc, &p.ProposedContent, &p.Status, &p.CreatedAt, &p.ResolvedAt, &p.ResolvedBy,
	)
	if err != nil {
		return FileEditProposal{}, err
	}
	if bc.Valid {
		s := bc.String
		p.BaseContent = &s
	}
	return p, nil
}

func (s *Store) ListFileEditProposalsForNode(ctx context.Context, orgID, spaceID, nodeID uuid.UUID) ([]FileEditProposal, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, space_id, node_id, author_user_id,
			base_content_sha256, base_content, proposed_content, status::text, created_at, resolved_at, resolved_by
		FROM file_edit_proposals
		WHERE organization_id = $1 AND space_id = $2 AND node_id = $3
		ORDER BY created_at DESC
	`, orgID, spaceID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposals(rows)
}

func scanProposals(rows pgx.Rows) ([]FileEditProposal, error) {
	var out []FileEditProposal
	for rows.Next() {
		var p FileEditProposal
		var bc sql.NullString
		if err := rows.Scan(
			&p.ID, &p.OrganizationID, &p.SpaceID, &p.NodeID, &p.AuthorUserID,
			&p.BaseContentSHA256, &bc, &p.ProposedContent, &p.Status, &p.CreatedAt, &p.ResolvedAt, &p.ResolvedBy,
		); err != nil {
			return nil, err
		}
		if bc.Valid {
			s := bc.String
			p.BaseContent = &s
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ResolveFileEditProposalRejected sets status to rejected.
func (s *Store) ResolveFileEditProposalRejected(ctx context.Context, orgID, proposalID, resolverID uuid.UUID) error {
	ct, err := s.Pool.Exec(ctx, `
		UPDATE file_edit_proposals
		SET status = 'rejected', resolved_at = now(), resolved_by = $3
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
	`, proposalID, orgID, resolverID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrProposalNotPending
	}
	return nil
}

// MarkProposalAccepted updates proposal row after successful file write (caller verifies hash).
func (s *Store) MarkProposalAccepted(ctx context.Context, orgID, proposalID, resolverID uuid.UUID) error {
	ct, err := s.Pool.Exec(ctx, `
		UPDATE file_edit_proposals
		SET status = 'accepted', resolved_at = now(), resolved_by = $3
		WHERE id = $1 AND organization_id = $2 AND status = 'pending'
	`, proposalID, orgID, resolverID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrProposalNotPending
	}
	return nil
}
