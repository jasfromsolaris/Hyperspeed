package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// SpaceAutomation is a workflow definition scoped to a space.
type SpaceAutomation struct {
	ID                        uuid.UUID       `json:"id"`
	OrganizationID            uuid.UUID       `json:"organization_id"`
	SpaceID                   uuid.UUID       `json:"space_id"`
	Name                      string          `json:"name"`
	Kind                      string          `json:"kind"`
	Config                    json.RawMessage `json:"config"`
	Status                    string          `json:"status"`
	CreatedByUserID           *uuid.UUID      `json:"created_by_user_id,omitempty"`
	CreatedByServiceAccountID *uuid.UUID      `json:"created_by_service_account_id,omitempty"`
	ReviewedByUserID          *uuid.UUID      `json:"reviewed_by_user_id,omitempty"`
	ReviewedAt                *time.Time      `json:"reviewed_at,omitempty"`
	RejectionReason           *string         `json:"rejection_reason,omitempty"`
	LastRunAt                 *time.Time      `json:"last_run_at,omitempty"`
	LastError                 *string         `json:"last_error,omitempty"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`
}

// SpaceAutomationRun is one execution attempt.
type SpaceAutomationRun struct {
	ID           uuid.UUID  `json:"id"`
	AutomationID uuid.UUID  `json:"automation_id"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Success      bool       `json:"success"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ExternalRef  *string    `json:"external_ref,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CreateSpaceAutomationInput inserts a new automation row.
type CreateSpaceAutomationInput struct {
	Name                      string
	Kind                      string
	Config                    json.RawMessage
	Status                    string
	OAuthTokenEnc             *string
	CreatedByUserID           *uuid.UUID
	CreatedByServiceAccountID *uuid.UUID
}

func scanSpaceAutomation(row interface {
	Scan(dest ...any) error
}) (SpaceAutomation, error) {
	var a SpaceAutomation
	var reviewedAt pgtype.Timestamptz
	var rej, lastErr pgtype.Text
	var lastRun pgtype.Timestamptz
	var cu, csa, ru pgtype.UUID
	err := row.Scan(
		&a.ID, &a.OrganizationID, &a.SpaceID, &a.Name, &a.Kind, &a.Config, &a.Status,
		&cu, &csa, &ru, &reviewedAt, &rej, &lastRun, &lastErr,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return a, err
	}
	if cu.Valid {
		v := uuid.UUID(cu.Bytes)
		a.CreatedByUserID = &v
	}
	if csa.Valid {
		v := uuid.UUID(csa.Bytes)
		a.CreatedByServiceAccountID = &v
	}
	if ru.Valid {
		v := uuid.UUID(ru.Bytes)
		a.ReviewedByUserID = &v
	}
	if reviewedAt.Valid {
		t := reviewedAt.Time
		a.ReviewedAt = &t
	}
	if rej.Valid {
		s := rej.String
		a.RejectionReason = &s
	}
	if lastRun.Valid {
		t := lastRun.Time
		a.LastRunAt = &t
	}
	if lastErr.Valid {
		s := lastErr.String
		a.LastError = &s
	}
	return a, nil
}

const spaceAutomationSelect = `
		SELECT id, organization_id, space_id, name, kind, config, status,
		       created_by_user_id, created_by_service_account_id,
		       reviewed_by_user_id, reviewed_at, rejection_reason,
		       last_run_at, last_error, created_at, updated_at
		FROM space_automations`

func (s *Store) ListSpaceAutomations(ctx context.Context, orgID, spaceID uuid.UUID) ([]SpaceAutomation, error) {
	rows, err := s.Pool.Query(ctx, spaceAutomationSelect+`
		WHERE organization_id = $1 AND space_id = $2
		ORDER BY updated_at DESC
	`, orgID, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceAutomation
	for rows.Next() {
		a, err := scanSpaceAutomation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetSpaceAutomation(ctx context.Context, orgID, spaceID, id uuid.UUID) (SpaceAutomation, error) {
	a, err := scanSpaceAutomation(s.Pool.QueryRow(ctx, spaceAutomationSelect+`
		WHERE organization_id = $1 AND space_id = $2 AND id = $3
	`, orgID, spaceID, id))
	return a, err
}

// GetSpaceAutomationOAuthToken returns encrypted blob (caller decrypts) or error.
func (s *Store) GetSpaceAutomationOAuthTokenEnc(ctx context.Context, orgID, spaceID, id uuid.UUID) (*string, error) {
	var enc *string
	err := s.Pool.QueryRow(ctx, `
		SELECT oauth_token_enc FROM space_automations
		WHERE organization_id = $1 AND space_id = $2 AND id = $3
	`, orgID, spaceID, id).Scan(&enc)
	if err != nil {
		return nil, err
	}
	if enc == nil || *enc == "" {
		return nil, errors.New("no oauth token")
	}
	return enc, nil
}

func (s *Store) CreateSpaceAutomation(ctx context.Context, orgID, spaceID uuid.UUID, in CreateSpaceAutomationInput) (SpaceAutomation, error) {
	if in.Config == nil {
		in.Config = json.RawMessage(`{}`)
	}
	var newID uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO space_automations (
			organization_id, space_id, name, kind, config, status, oauth_token_enc,
			created_by_user_id, created_by_service_account_id
		) VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9)
		RETURNING id
	`, orgID, spaceID, in.Name, in.Kind, string(in.Config), in.Status, in.OAuthTokenEnc,
		in.CreatedByUserID, in.CreatedByServiceAccountID,
	).Scan(&newID)
	if err != nil {
		return SpaceAutomation{}, err
	}
	return s.GetSpaceAutomation(ctx, orgID, spaceID, newID)
}

type PatchSpaceAutomationInput struct {
	Name          *string
	Config        json.RawMessage
	Status        *string
	OAuthTokenEnc *string
	LastError     *string
	LastRunAt     *time.Time
}

func (s *Store) PatchSpaceAutomation(ctx context.Context, orgID, spaceID, id uuid.UUID, in PatchSpaceAutomationInput) (SpaceAutomation, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE space_automations SET
			name = COALESCE($4, name),
			config = COALESCE($5::jsonb, config),
			status = COALESCE($6, status),
			oauth_token_enc = COALESCE($7, oauth_token_enc),
			last_error = COALESCE($8, last_error),
			last_run_at = COALESCE($9, last_run_at),
			updated_at = now()
		WHERE organization_id = $1 AND space_id = $2 AND id = $3
	`, orgID, spaceID, id, in.Name, nullableJSON(in.Config), in.Status, in.OAuthTokenEnc, in.LastError, in.LastRunAt)
	if err != nil {
		return SpaceAutomation{}, err
	}
	if tag.RowsAffected() == 0 {
		return SpaceAutomation{}, pgx.ErrNoRows
	}
	return s.GetSpaceAutomation(ctx, orgID, spaceID, id)
}

func nullableJSON(j json.RawMessage) interface{} {
	if j == nil {
		return nil
	}
	return string(j)
}

func (s *Store) ApproveSpaceAutomation(ctx context.Context, orgID, spaceID, id, reviewer uuid.UUID) (SpaceAutomation, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE space_automations SET
			status = 'active',
			reviewed_by_user_id = $4,
			reviewed_at = now(),
			rejection_reason = NULL,
			updated_at = now()
		WHERE organization_id = $1 AND space_id = $2 AND id = $3 AND status = 'pending_approval'
	`, orgID, spaceID, id, reviewer)
	if err != nil {
		return SpaceAutomation{}, err
	}
	if tag.RowsAffected() == 0 {
		return SpaceAutomation{}, pgx.ErrNoRows
	}
	return s.GetSpaceAutomation(ctx, orgID, spaceID, id)
}

func (s *Store) RejectSpaceAutomation(ctx context.Context, orgID, spaceID, id, reviewer uuid.UUID, reason string) (SpaceAutomation, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE space_automations SET
			status = 'rejected',
			reviewed_by_user_id = $4,
			reviewed_at = now(),
			rejection_reason = $5,
			updated_at = now()
		WHERE organization_id = $1 AND space_id = $2 AND id = $3 AND status = 'pending_approval'
	`, orgID, spaceID, id, reviewer, reason)
	if err != nil {
		return SpaceAutomation{}, err
	}
	if tag.RowsAffected() == 0 {
		return SpaceAutomation{}, pgx.ErrNoRows
	}
	return s.GetSpaceAutomation(ctx, orgID, spaceID, id)
}

func (s *Store) DeleteSpaceAutomation(ctx context.Context, orgID, spaceID, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM space_automations WHERE organization_id = $1 AND space_id = $2 AND id = $3
	`, orgID, spaceID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) InsertAutomationRun(ctx context.Context, automationID uuid.UUID, success bool, errMsg, externalRef *string) (SpaceAutomationRun, error) {
	var r SpaceAutomationRun
	var fin pgtype.Timestamptz
	var em, er pgtype.Text
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO space_automation_runs (automation_id, finished_at, success, error_message, external_ref)
		VALUES ($1, now(), $2, $3, $4)
		RETURNING id, automation_id, started_at, finished_at, success, error_message, external_ref, created_at
	`, automationID, success, errMsg, externalRef,
	).Scan(&r.ID, &r.AutomationID, &r.StartedAt, &fin, &r.Success, &em, &er, &r.CreatedAt)
	if err != nil {
		return r, err
	}
	if fin.Valid {
		t := fin.Time
		r.FinishedAt = &t
	}
	if em.Valid {
		s := em.String
		r.ErrorMessage = &s
	}
	if er.Valid {
		s := er.String
		r.ExternalRef = &s
	}
	return r, nil
}

func (s *Store) ListAutomationRuns(ctx context.Context, orgID, spaceID, automationID uuid.UUID, limit int) ([]SpaceAutomationRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.automation_id, r.started_at, r.finished_at, r.success, r.error_message, r.external_ref, r.created_at
		FROM space_automation_runs r
		INNER JOIN space_automations a ON a.id = r.automation_id
		WHERE a.organization_id = $1 AND a.space_id = $2 AND r.automation_id = $3
		ORDER BY r.started_at DESC
		LIMIT $4
	`, orgID, spaceID, automationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceAutomationRun
	for rows.Next() {
		var r SpaceAutomationRun
		var fin pgtype.Timestamptz
		var em, er pgtype.Text
		if err := rows.Scan(&r.ID, &r.AutomationID, &r.StartedAt, &fin, &r.Success, &em, &er, &r.CreatedAt); err != nil {
			return nil, err
		}
		if fin.Valid {
			t := fin.Time
			r.FinishedAt = &t
		}
		if em.Valid {
			s := em.String
			r.ErrorMessage = &s
		}
		if er.Valid {
			s := er.String
			r.ExternalRef = &s
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) TouchSpaceAutomationLastRun(ctx context.Context, orgID, spaceID, id uuid.UUID, at time.Time, lastErr *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE space_automations SET last_run_at = $4, last_error = $5, updated_at = now()
		WHERE organization_id = $1 AND space_id = $2 AND id = $3
	`, orgID, spaceID, id, at, lastErr)
	return err
}
