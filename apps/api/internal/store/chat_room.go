package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ChatRoom struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"space_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListChatRooms(ctx context.Context, projectID uuid.UUID) ([]ChatRoom, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, name, created_at FROM chat_rooms
		WHERE space_id = $1 ORDER BY created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatRoom
	for rows.Next() {
		var r ChatRoom
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateChatRoom(ctx context.Context, projectID uuid.UUID, name string) (ChatRoom, error) {
	var r ChatRoom
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO chat_rooms (space_id, name) VALUES ($1, $2)
		RETURNING id, space_id, name, created_at
	`, projectID, name).Scan(&r.ID, &r.ProjectID, &r.Name, &r.CreatedAt)
	return r, err
}

func (s *Store) GetChatRoomInSpace(ctx context.Context, spaceID, chatRoomID uuid.UUID) (ChatRoom, error) {
	var r ChatRoom
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, name, created_at FROM chat_rooms
		WHERE id = $1 AND space_id = $2
	`, chatRoomID, spaceID).Scan(&r.ID, &r.ProjectID, &r.Name, &r.CreatedAt)
	if err != nil {
		return ChatRoom{}, err
	}
	return r, nil
}

func (s *Store) DeleteChatRoom(ctx context.Context, projectID, chatRoomID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM chat_rooms WHERE id = $1 AND space_id = $2
	`, chatRoomID, projectID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
