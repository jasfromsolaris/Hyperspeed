package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SpaceMemberRole string

const (
	SpaceRoleOwner  SpaceMemberRole = "owner"
	SpaceRoleMember SpaceMemberRole = "member"
)

type SpaceMember struct {
	SpaceID  uuid.UUID      `json:"space_id"`
	UserID   uuid.UUID      `json:"user_id"`
	Role     SpaceMemberRole `json:"role"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *Store) AddSpaceMember(ctx context.Context, spaceID, userID uuid.UUID, role SpaceMemberRole) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO space_members (space_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (space_id, user_id) DO UPDATE SET role = EXCLUDED.role
	`, spaceID, userID, role)
	return err
}

func (s *Store) SpaceMemberRole(ctx context.Context, spaceID, userID uuid.UUID) (SpaceMemberRole, error) {
	var r SpaceMemberRole
	err := s.Pool.QueryRow(ctx, `
		SELECT role FROM space_members WHERE space_id = $1 AND user_id = $2
	`, spaceID, userID).Scan(&r)
	if err == pgx.ErrNoRows {
		return "", pgx.ErrNoRows
	}
	return r, err
}

