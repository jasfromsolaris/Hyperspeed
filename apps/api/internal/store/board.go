package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Board struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"space_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type BoardColumn struct {
	ID       uuid.UUID `json:"id"`
	BoardID  uuid.UUID `json:"board_id"`
	Name     string    `json:"name"`
	Position int       `json:"position"`
}

// GetBoardByProject returns the first board for a project (legacy / convenience).
func (s *Store) GetBoardByProject(ctx context.Context, projectID uuid.UUID) (Board, error) {
	var b Board
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, name, created_at FROM boards
		WHERE space_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, projectID).Scan(&b.ID, &b.ProjectID, &b.Name, &b.CreatedAt)
	return b, err
}

func (s *Store) GetBoardInProject(ctx context.Context, projectID, boardID uuid.UUID) (Board, error) {
	var b Board
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, name, created_at FROM boards
		WHERE id = $1 AND space_id = $2
	`, boardID, projectID).Scan(&b.ID, &b.ProjectID, &b.Name, &b.CreatedAt)
	return b, err
}

func (s *Store) ListBoardsByProject(ctx context.Context, projectID uuid.UUID) ([]Board, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, name, created_at FROM boards
		WHERE space_id = $1
		ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBoardInProject(ctx context.Context, projectID, boardID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM boards WHERE id = $1 AND space_id = $2
	`, boardID, projectID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) CreateBoardWithDefaultColumns(ctx context.Context, projectID uuid.UUID, name string) (Board, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Board{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if name == "" {
		name = "Task board"
	}

	var b Board
	err = tx.QueryRow(ctx, `
		INSERT INTO boards (space_id, name) VALUES ($1, $2)
		RETURNING id, space_id, name, created_at
	`, projectID, name).Scan(&b.ID, &b.ProjectID, &b.Name, &b.CreatedAt)
	if err != nil {
		return Board{}, err
	}

	cols := []struct {
		Name     string
		Position int
	}{
		{"To do", 0},
		{"In progress", 1},
		{"Done", 2},
		{"Overdue", 3},
	}
	for _, c := range cols {
		_, err = tx.Exec(ctx, `
			INSERT INTO board_columns (board_id, name, position) VALUES ($1, $2, $3)
		`, b.ID, c.Name, c.Position)
		if err != nil {
			return Board{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Board{}, err
	}
	return b, nil
}

func (s *Store) ListColumns(ctx context.Context, boardID uuid.UUID) ([]BoardColumn, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, board_id, name, position FROM board_columns
		WHERE board_id = $1 ORDER BY position ASC, name ASC
	`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BoardColumn
	for rows.Next() {
		var c BoardColumn
		if err := rows.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AddColumn(ctx context.Context, boardID uuid.UUID, name string, position int) (BoardColumn, error) {
	var c BoardColumn
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO board_columns (board_id, name, position)
		VALUES ($1, $2, $3)
		RETURNING id, board_id, name, position
	`, boardID, name, position).Scan(&c.ID, &c.BoardID, &c.Name, &c.Position)
	return c, err
}
