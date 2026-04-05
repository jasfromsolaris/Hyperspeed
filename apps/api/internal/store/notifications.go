package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Notification struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	UserID         uuid.UUID       `json:"user_id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"created_at"`
	ReadAt         *time.Time      `json:"read_at,omitempty"`
}

func (s *Store) CreateNotification(ctx context.Context, orgID, userID uuid.UUID, typ string, payload []byte) (Notification, error) {
	var n Notification
	if payload == nil {
		payload = []byte(`{}`)
	}
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, user_id, type, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization_id, user_id, type, payload, created_at, read_at
	`, orgID, userID, typ, payload).Scan(
		&n.ID, &n.OrganizationID, &n.UserID, &n.Type, &n.Payload, &n.CreatedAt, &n.ReadAt,
	)
	return n, err
}

func (s *Store) ListNotificationsForUser(ctx context.Context, orgID, userID uuid.UUID, limit int, before *time.Time) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var (
		rows pgx.Rows
		err  error
	)
	if before != nil {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, organization_id, user_id, type, payload, created_at, read_at
			FROM notifications
			WHERE organization_id = $1
			  AND user_id = $2
			  AND created_at < $3
			ORDER BY created_at DESC
			LIMIT $4
		`, orgID, userID, *before, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, organization_id, user_id, type, payload, created_at, read_at
			FROM notifications
			WHERE organization_id = $1
			  AND user_id = $2
			ORDER BY created_at DESC
			LIMIT $3
		`, orgID, userID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.OrganizationID, &n.UserID, &n.Type, &n.Payload, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListNotificationIDsByChatTarget(ctx context.Context, orgID, userID uuid.UUID, typ string, spaceID, chatRoomID uuid.UUID, unreadOnly bool, limit int) ([]uuid.UUID, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows pgx.Rows
	var err error
	if unreadOnly {
		rows, err = s.Pool.Query(ctx, `
			SELECT id
			FROM notifications
			WHERE organization_id = $1
			  AND user_id = $2
			  AND type = $3
			  AND read_at IS NULL
			  AND (payload->>'space_id') = $4
			  AND (payload->>'chat_room_id') = $5
			ORDER BY created_at DESC
			LIMIT $6
		`, orgID, userID, typ, spaceID.String(), chatRoomID.String(), limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id
			FROM notifications
			WHERE organization_id = $1
			  AND user_id = $2
			  AND type = $3
			  AND (payload->>'space_id') = $4
			  AND (payload->>'chat_room_id') = $5
			ORDER BY created_at DESC
			LIMIT $6
		`, orgID, userID, typ, spaceID.String(), chatRoomID.String(), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) UnreadNotificationsCount(ctx context.Context, orgID, userID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notifications
		WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
	`, orgID, userID).Scan(&n)
	return n, err
}

func (s *Store) MarkNotificationsRead(ctx context.Context, orgID, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	// If ids empty, mark all as read.
	if len(ids) == 0 {
		tag, err := s.Pool.Exec(ctx, `
			UPDATE notifications
			SET read_at = now()
			WHERE organization_id = $1 AND user_id = $2 AND read_at IS NULL
		`, orgID, userID)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE notifications
		SET read_at = now()
		WHERE organization_id = $1
		  AND user_id = $2
		  AND id = ANY($3)
		  AND read_at IS NULL
	`, orgID, userID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteNotifications removes rows owned by the user in the org. ids must be non-empty.
func (s *Store) DeleteNotifications(ctx context.Context, orgID, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM notifications
		WHERE organization_id = $1
		  AND user_id = $2
		  AND id = ANY($3)
	`, orgID, userID, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

