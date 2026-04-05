package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChatMessage struct {
	ID         uuid.UUID       `json:"id"`
	ChatRoomID uuid.UUID       `json:"chat_room_id"`
	ProjectID  uuid.UUID       `json:"space_id"`
	AuthorID   *uuid.UUID      `json:"author_user_id"`
	Content    string          `json:"content"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	EditedAt   *time.Time      `json:"edited_at,omitempty"`
	DeletedAt  *time.Time      `json:"deleted_at,omitempty"`
}

type ChatMessageReaction struct {
	MessageID uuid.UUID `json:"message_id"`
	UserID    uuid.UUID `json:"user_id"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessageAttachment struct {
	ID        uuid.UUID `json:"id"`
	MessageID uuid.UUID `json:"message_id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	SizeBytes int       `json:"size_bytes"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type ChatMessageWithMeta struct {
	Message     ChatMessage              `json:"message"`
	Reactions   []ChatMessageReaction    `json:"reactions"`
	Attachments []ChatMessageAttachment  `json:"attachments"`
}

func (s *Store) ListChatMessages(ctx context.Context, chatRoomID uuid.UUID, limit int, before *time.Time) ([]ChatMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var (
		rows pgx.Rows
		err  error
	)
	if before != nil {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
			FROM chat_messages
			WHERE chat_room_id = $1 AND created_at < $2
			ORDER BY created_at DESC
			LIMIT $3
		`, chatRoomID, *before, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
			FROM chat_messages
			WHERE chat_room_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, chatRoomID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(
			&m.ID,
			&m.ChatRoomID,
			&m.ProjectID,
			&m.AuthorID,
			&m.Content,
			&m.Metadata,
			&m.CreatedAt,
			&m.UpdatedAt,
			&m.EditedAt,
			&m.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Keep chronological for UI.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) CreateChatMessage(ctx context.Context, projectID, chatRoomID uuid.UUID, authorUserID uuid.UUID, content string) (ChatMessage, error) {
	content = strings.TrimSpace(content)
	var m ChatMessage
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO chat_messages (chat_room_id, space_id, author_user_id, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
	`, chatRoomID, projectID, authorUserID, content).Scan(
		&m.ID,
		&m.ChatRoomID,
		&m.ProjectID,
		&m.AuthorID,
		&m.Content,
		&m.Metadata,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.EditedAt,
		&m.DeletedAt,
	)
	return m, err
}

// CreateChatMessageWithMetadata inserts a chat message with optional JSON metadata (e.g. agent run cards).
func (s *Store) CreateChatMessageWithMetadata(ctx context.Context, projectID, chatRoomID, authorUserID uuid.UUID, content string, metadata json.RawMessage) (ChatMessage, error) {
	content = strings.TrimSpace(content)
	var m ChatMessage
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO chat_messages (chat_room_id, space_id, author_user_id, content, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
	`, chatRoomID, projectID, authorUserID, content, metadata).Scan(
		&m.ID,
		&m.ChatRoomID,
		&m.ProjectID,
		&m.AuthorID,
		&m.Content,
		&m.Metadata,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.EditedAt,
		&m.DeletedAt,
	)
	return m, err
}

func (s *Store) UpdateChatMessageContent(ctx context.Context, projectID, chatRoomID, messageID, actorUserID uuid.UUID, content string) (ChatMessage, error) {
	content = strings.TrimSpace(content)
	var m ChatMessage
	now := time.Now()
	err := s.Pool.QueryRow(ctx, `
		UPDATE chat_messages
		SET content = $1, updated_at = now(), edited_at = $2
		WHERE id = $3 AND chat_room_id = $4 AND space_id = $5 AND author_user_id = $6 AND deleted_at IS NULL
		RETURNING id, chat_room_id, space_id, author_user_id, content, created_at, updated_at, edited_at, deleted_at
	`, content, now, messageID, chatRoomID, projectID, actorUserID).Scan(
		&m.ID,
		&m.ChatRoomID,
		&m.ProjectID,
		&m.AuthorID,
		&m.Content,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.EditedAt,
		&m.DeletedAt,
	)
	return m, err
}

func (s *Store) SoftDeleteChatMessage(ctx context.Context, projectID, chatRoomID, messageID, actorUserID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE chat_messages
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND chat_room_id = $2 AND space_id = $3 AND author_user_id = $4 AND deleted_at IS NULL
	`, messageID, chatRoomID, projectID, actorUserID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		return true, nil
	}
	// Remove any message in this room (AI, other members, NULL author). REST requires ChatWrite + space membership.
	return s.softDeleteChatMessageModerator(ctx, projectID, chatRoomID, messageID)
}

func (s *Store) softDeleteChatMessageModerator(ctx context.Context, projectID, chatRoomID, messageID uuid.UUID) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE chat_messages
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND chat_room_id = $2 AND space_id = $3 AND deleted_at IS NULL
	`, messageID, chatRoomID, projectID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) AddReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string) (ChatMessageReaction, error) {
	emoji = strings.TrimSpace(emoji)
	var r ChatMessageReaction
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO chat_message_reactions (message_id, user_id, emoji)
		VALUES ($1, $2, $3)
		ON CONFLICT (message_id, user_id, emoji) DO UPDATE SET created_at = now()
		RETURNING message_id, user_id, emoji, created_at
	`, messageID, userID, emoji).Scan(&r.MessageID, &r.UserID, &r.Emoji, &r.CreatedAt)
	return r, err
}

func (s *Store) RemoveReaction(ctx context.Context, messageID, userID uuid.UUID, emoji string) (bool, error) {
	emoji = strings.TrimSpace(emoji)
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM chat_message_reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3
	`, messageID, userID, emoji)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ListReactionsForMessages(ctx context.Context, messageIDs []uuid.UUID) ([]ChatMessageReaction, error) {
	if len(messageIDs) == 0 {
		return []ChatMessageReaction{}, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT message_id, user_id, emoji, created_at
		FROM chat_message_reactions
		WHERE message_id = ANY($1)
	`, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessageReaction
	for rows.Next() {
		var r ChatMessageReaction
		if err := rows.Scan(&r.MessageID, &r.UserID, &r.Emoji, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) AddAttachment(ctx context.Context, messageID uuid.UUID, name, mime string, sizeBytes int, url string) (ChatMessageAttachment, error) {
	name = strings.TrimSpace(name)
	mime = strings.TrimSpace(mime)
	url = strings.TrimSpace(url)
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	var a ChatMessageAttachment
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO chat_message_attachments (message_id, name, mime, size_bytes, url)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, message_id, name, mime, size_bytes, url, created_at
	`, messageID, name, mime, sizeBytes, url).Scan(
		&a.ID, &a.MessageID, &a.Name, &a.Mime, &a.SizeBytes, &a.URL, &a.CreatedAt,
	)
	return a, err
}

func (s *Store) ListAttachmentsForMessages(ctx context.Context, messageIDs []uuid.UUID) ([]ChatMessageAttachment, error) {
	if len(messageIDs) == 0 {
		return []ChatMessageAttachment{}, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, message_id, name, mime, size_bytes, url, created_at
		FROM chat_message_attachments
		WHERE message_id = ANY($1)
		ORDER BY created_at ASC
	`, messageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessageAttachment
	for rows.Next() {
		var a ChatMessageAttachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Name, &a.Mime, &a.SizeBytes, &a.URL, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SearchChatMessages(ctx context.Context, chatRoomID uuid.UUID, q string, limit int) ([]ChatMessage, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []ChatMessage{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
		FROM chat_messages
		WHERE chat_room_id = $1
		  AND deleted_at IS NULL
		  AND to_tsvector('english', content) @@ plainto_tsquery('english', $2)
		ORDER BY created_at DESC
		LIMIT $3
	`, chatRoomID, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(
			&m.ID,
			&m.ChatRoomID,
			&m.ProjectID,
			&m.AuthorID,
			&m.Content,
			&m.Metadata,
			&m.CreatedAt,
			&m.UpdatedAt,
			&m.EditedAt,
			&m.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

