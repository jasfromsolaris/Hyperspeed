package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TaskMessage struct {
	ID            uuid.UUID  `json:"id"`
	SpaceID       uuid.UUID  `json:"space_id"`
	TaskID        uuid.UUID  `json:"task_id"`
	AuthorUserID  *uuid.UUID `json:"author_user_id,omitempty"`
	Content       string     `json:"content"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (s *Store) ListTaskMessages(ctx context.Context, spaceID, taskID uuid.UUID, limit int, before *time.Time) ([]TaskMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	if before != nil {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, space_id, task_id, author_user_id, content, created_at
			FROM task_messages
			WHERE space_id = $1 AND task_id = $2 AND created_at < $3
			ORDER BY created_at DESC
			LIMIT $4
		`, spaceID, taskID, *before, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, space_id, task_id, author_user_id, content, created_at
			FROM task_messages
			WHERE space_id = $1 AND task_id = $2
			ORDER BY created_at DESC
			LIMIT $3
		`, spaceID, taskID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskMessage
	for rows.Next() {
		var m TaskMessage
		if err := rows.Scan(&m.ID, &m.SpaceID, &m.TaskID, &m.AuthorUserID, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	// Return chronological order for UI.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

func (s *Store) InsertTaskMessage(ctx context.Context, spaceID, taskID, authorID uuid.UUID, content string) (TaskMessage, error) {
	var m TaskMessage
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO task_messages (space_id, task_id, author_user_id, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, space_id, task_id, author_user_id, content, created_at
	`, spaceID, taskID, authorID, content).Scan(
		&m.ID, &m.SpaceID, &m.TaskID, &m.AuthorUserID, &m.Content, &m.CreatedAt,
	)
	return m, err
}
