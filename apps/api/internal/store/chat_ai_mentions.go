package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChatAIMentionReplyRecord struct {
	ID                uuid.UUID       `json:"id"`
	OrganizationID    uuid.UUID       `json:"organization_id"`
	SpaceID           uuid.UUID       `json:"space_id"`
	ChatRoomID        uuid.UUID       `json:"chat_room_id"`
	SourceMessageID   uuid.UUID       `json:"source_message_id"`
	AIUserID          uuid.UUID       `json:"ai_user_id"`
	RequestedByUserID uuid.UUID       `json:"requested_by_user_id"`
	ResponseMessageID *uuid.UUID      `json:"response_message_id,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	RespondedAt       *time.Time      `json:"responded_at,omitempty"`
	RunDetail         json.RawMessage `json:"run_detail,omitempty"`
}

func (s *Store) GetChatMessageByID(ctx context.Context, spaceID, chatRoomID, messageID uuid.UUID) (ChatMessage, error) {
	var m ChatMessage
	err := s.Pool.QueryRow(ctx, `
		SELECT id, chat_room_id, space_id, author_user_id, content, metadata, created_at, updated_at, edited_at, deleted_at
		FROM chat_messages
		WHERE id = $1 AND chat_room_id = $2 AND space_id = $3
	`, messageID, chatRoomID, spaceID).Scan(
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

// CreateChatAIMentionReplyRecord inserts a dedupe record; inserted=false on duplicate.
func (s *Store) CreateChatAIMentionReplyRecord(ctx context.Context, orgID, spaceID, chatRoomID, sourceMessageID, aiUserID, requestedBy uuid.UUID) (inserted bool, rec ChatAIMentionReplyRecord, err error) {
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO chat_ai_mention_replies (
			organization_id, space_id, chat_room_id, source_message_id, ai_user_id, requested_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_message_id, ai_user_id) DO NOTHING
		RETURNING id, organization_id, space_id, chat_room_id, source_message_id, ai_user_id, requested_by_user_id, response_message_id, created_at, responded_at
	`, orgID, spaceID, chatRoomID, sourceMessageID, aiUserID, requestedBy).Scan(
		&rec.ID,
		&rec.OrganizationID,
		&rec.SpaceID,
		&rec.ChatRoomID,
		&rec.SourceMessageID,
		&rec.AIUserID,
		&rec.RequestedByUserID,
		&rec.ResponseMessageID,
		&rec.CreatedAt,
		&rec.RespondedAt,
	)
	if err == pgx.ErrNoRows {
		return false, ChatAIMentionReplyRecord{}, nil
	}
	if err != nil {
		return false, ChatAIMentionReplyRecord{}, err
	}
	return true, rec, nil
}

func (s *Store) MarkChatAIMentionReplyResponded(ctx context.Context, sourceMessageID, aiUserID, responseMessageID uuid.UUID, runDetail []byte) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE chat_ai_mention_replies
		SET response_message_id = $3, responded_at = now(),
		    run_detail = COALESCE($4::jsonb, run_detail)
		WHERE source_message_id = $1 AND ai_user_id = $2
	`, sourceMessageID, aiUserID, responseMessageID, runDetail)
	return err
}

// ChatAIMentionReplyEnriched is a mention-reply row with space and room display names.
type ChatAIMentionReplyEnriched struct {
	ChatAIMentionReplyRecord
	SpaceName    string `json:"space_name"`
	ChatRoomName string `json:"chat_room_name"`
}

// GetChatAIMentionReplyEnrichedByID returns one mention-reply row with names and run_detail (Peek detail).
func (s *Store) GetChatAIMentionReplyEnrichedByID(ctx context.Context, orgID, replyID uuid.UUID) (ChatAIMentionReplyEnriched, error) {
	var e ChatAIMentionReplyEnriched
	err := s.Pool.QueryRow(ctx, `
		SELECT r.id, r.organization_id, r.space_id, r.chat_room_id, r.source_message_id, r.ai_user_id,
		       r.requested_by_user_id, r.response_message_id, r.created_at, r.responded_at, r.run_detail,
		       sp.name, cr.name
		FROM chat_ai_mention_replies r
		JOIN spaces sp ON sp.id = r.space_id AND sp.organization_id = r.organization_id
		JOIN chat_rooms cr ON cr.id = r.chat_room_id AND cr.space_id = r.space_id
		WHERE r.organization_id = $1 AND r.id = $2
	`, orgID, replyID).Scan(
		&e.ID,
		&e.OrganizationID,
		&e.SpaceID,
		&e.ChatRoomID,
		&e.SourceMessageID,
		&e.AIUserID,
		&e.RequestedByUserID,
		&e.ResponseMessageID,
		&e.CreatedAt,
		&e.RespondedAt,
		&e.RunDetail,
		&e.SpaceName,
		&e.ChatRoomName,
	)
	return e, err
}

// ListChatAIMentionRepliesEnrichedForOrg returns recent AI mention reply records for an organization
// with space and chat room names (newest first). Caller applies access filters.
func (s *Store) ListChatAIMentionRepliesEnrichedForOrg(ctx context.Context, orgID uuid.UUID, limit int) ([]ChatAIMentionReplyEnriched, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.organization_id, r.space_id, r.chat_room_id, r.source_message_id, r.ai_user_id,
		       r.requested_by_user_id, r.response_message_id, r.created_at, r.responded_at,
		       sp.name, cr.name
		FROM chat_ai_mention_replies r
		JOIN spaces sp ON sp.id = r.space_id AND sp.organization_id = r.organization_id
		JOIN chat_rooms cr ON cr.id = r.chat_room_id AND cr.space_id = r.space_id
		WHERE r.organization_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2
	`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChatAIMentionReplyEnriched
	for rows.Next() {
		var e ChatAIMentionReplyEnriched
		if err := rows.Scan(
			&e.ID,
			&e.OrganizationID,
			&e.SpaceID,
			&e.ChatRoomID,
			&e.SourceMessageID,
			&e.AIUserID,
			&e.RequestedByUserID,
			&e.ResponseMessageID,
			&e.CreatedAt,
			&e.RespondedAt,
			&e.SpaceName,
			&e.ChatRoomName,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
