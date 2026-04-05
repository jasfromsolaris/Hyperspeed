package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PreviewSessionStatus string

const (
	PreviewSessionPending  PreviewSessionStatus = "pending"
	PreviewSessionRunning  PreviewSessionStatus = "running"
	PreviewSessionFailed   PreviewSessionStatus = "failed"
	PreviewSessionExpired  PreviewSessionStatus = "expired"
)

type PreviewSession struct {
	ID           uuid.UUID              `json:"id"`
	SpaceID      uuid.UUID              `json:"space_id"`
	CreatedBy    uuid.UUID              `json:"created_by"`
	Status       PreviewSessionStatus   `json:"status"`
	Command      *string                `json:"command,omitempty"`
	Cwd          *string                `json:"cwd,omitempty"`
	AccessToken  string                 `json:"-"`
	ErrorMessage *string                `json:"error_message,omitempty"`
	SnapshotJSON json.RawMessage        `json:"-"`
	ExpiresAt    time.Time              `json:"expires_at"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

func (s *Store) InsertPreviewSession(ctx context.Context, spaceID, createdBy uuid.UUID, status PreviewSessionStatus, command, cwd *string, accessToken string, snapshot map[string]string, expiresAt time.Time) (PreviewSession, error) {
	snapBytes, err := json.Marshal(snapshot)
	if err != nil {
		return PreviewSession{}, err
	}
	var row PreviewSession
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO preview_sessions (space_id, created_by, status, command, cwd, access_token, snapshot_json, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		RETURNING id, space_id, created_by, status, command, cwd, access_token, error_message, snapshot_json, expires_at, created_at, updated_at
	`, spaceID, createdBy, string(status), command, cwd, accessToken, snapBytes, expiresAt).Scan(
		&row.ID, &row.SpaceID, &row.CreatedBy, &row.Status, &row.Command, &row.Cwd, &row.AccessToken, &row.ErrorMessage, &row.SnapshotJSON, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return PreviewSession{}, err
	}
	return row, nil
}

func (s *Store) GetPreviewSession(ctx context.Context, spaceID, sessionID uuid.UUID) (PreviewSession, error) {
	var row PreviewSession
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, created_by, status, command, cwd, access_token, error_message, snapshot_json, expires_at, created_at, updated_at
		FROM preview_sessions
		WHERE id = $1 AND space_id = $2
	`, sessionID, spaceID).Scan(
		&row.ID, &row.SpaceID, &row.CreatedBy, &row.Status, &row.Command, &row.Cwd, &row.AccessToken, &row.ErrorMessage, &row.SnapshotJSON, &row.ExpiresAt, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return PreviewSession{}, err
	}
	return row, nil
}

func (s *Store) DeletePreviewSession(ctx context.Context, spaceID, sessionID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM preview_sessions WHERE id = $1 AND space_id = $2`, sessionID, spaceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) UpdatePreviewSessionStatus(ctx context.Context, spaceID, sessionID uuid.UUID, status PreviewSessionStatus, errMsg *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE preview_sessions SET status = $3, error_message = $4, updated_at = now()
		WHERE id = $1 AND space_id = $2
	`, sessionID, spaceID, string(status), errMsg)
	return err
}

// ExpireStalePreviewSessions marks old pending sessions as expired (optional cron).
func (s *Store) ExpireStalePreviewSessions(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE preview_sessions SET status = 'expired', updated_at = now()
		WHERE expires_at < now() AND status IN ('pending', 'running')
	`)
	return err
}
