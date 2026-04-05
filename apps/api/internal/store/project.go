package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Space struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *Store) CreateSpace(ctx context.Context, orgID uuid.UUID, name, description string) (Space, error) {
	var p Space
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO spaces (organization_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, organization_id, name, description, created_at
	`, orgID, name, description).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Description, &p.CreatedAt)
	return p, err
}

func (s *Store) ListSpaces(ctx context.Context, orgID uuid.UUID) ([]Space, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, name, description, created_at
		FROM spaces WHERE organization_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Space
	for rows.Next() {
		var p Space
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetSpace(ctx context.Context, orgID, spaceID uuid.UUID) (Space, error) {
	var p Space
	err := s.Pool.QueryRow(ctx, `
		SELECT id, organization_id, name, description, created_at
		FROM spaces WHERE id = $1 AND organization_id = $2
	`, spaceID, orgID).Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Description, &p.CreatedAt)
	return p, err
}
