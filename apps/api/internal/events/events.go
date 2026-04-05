package events

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const ChannelPrefix = "hyperspeed:org:"

func OrgChannel(orgID uuid.UUID) string {
	return ChannelPrefix + orgID.String()
}

type Type string

const (
	TaskCreated   Type = "task.created"
	TaskUpdated   Type = "task.updated"
	TaskDeleted   Type = "task.deleted"
	TaskMessageCreated Type = "task.message.created"
	ProjectUpdated Type = "project.updated"
	ChatMessageCreated Type = "chat.message.created"
	ChatMessageUpdated Type = "chat.message.updated"
	ChatMessageDeleted Type = "chat.message.deleted"
	ChatReactionAdded  Type = "chat.reaction.added"
	ChatReactionRemoved Type = "chat.reaction.removed"
	ChatAIMentionRequested Type = "chat.ai_mention.requested"
	NotificationCreated Type = "notification.created"
	// FileTreeUpdated signals space file tree / listing changed (rename, move, create, delete).
	FileTreeUpdated Type = "file.tree.updated"
)

type ChatAIMentionRequestedPayload struct {
	OrganizationID   uuid.UUID `json:"organization_id"`
	SpaceID          uuid.UUID `json:"space_id"`
	ChatRoomID       uuid.UUID `json:"chat_room_id"`
	SourceMessageID  uuid.UUID `json:"source_message_id"`
	AIUserID         uuid.UUID `json:"ai_user_id"`
	RequestedByUserID uuid.UUID `json:"requested_by_user_id"`
}

type Envelope struct {
	Type           Type            `json:"type"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	ProjectID      *uuid.UUID      `json:"project_id,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

func Marshal(t Type, orgID uuid.UUID, projectID *uuid.UUID, payload any) ([]byte, error) {
	p, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	env := Envelope{Type: t, OrganizationID: orgID, ProjectID: projectID, Payload: p}
	return json.Marshal(env)
}

func Parse(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("parse event: %w", err)
	}
	return env, nil
}

func UnmarshalPayload[T any](env Envelope, out *T) error {
	return json.Unmarshal(env.Payload, out)
}
