package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SpaceFile struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"space_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListSpaceFiles(ctx context.Context, projectID uuid.UUID) ([]SpaceFile, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, name, created_at FROM space_files
		WHERE space_id = $1 ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceFile
	for rows.Next() {
		var f SpaceFile
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateSpaceFile(ctx context.Context, projectID uuid.UUID, name string) (SpaceFile, error) {
	var f SpaceFile
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO space_files (space_id, name) VALUES ($1, $2)
		RETURNING id, space_id, name, created_at
	`, projectID, name).Scan(&f.ID, &f.ProjectID, &f.Name, &f.CreatedAt)
	return f, err
}

func (s *Store) DeleteSpaceFile(ctx context.Context, projectID, fileID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM space_files WHERE id = $1 AND space_id = $2
	`, fileID, projectID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
