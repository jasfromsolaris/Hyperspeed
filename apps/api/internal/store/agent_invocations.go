package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AgentToolInvocation struct {
	ID             uuid.UUID       `json:"id"`
	OrganizationID uuid.UUID       `json:"organization_id"`
	UserID         uuid.UUID       `json:"user_id"`
	SessionID      *string         `json:"session_id,omitempty"`
	Tool           string          `json:"tool"`
	ArgumentsJSON  json.RawMessage `json:"arguments,omitempty"`
	ResultJSON     json.RawMessage `json:"result,omitempty"`
	ErrorText      *string         `json:"error,omitempty"`
	DurationMs     *int            `json:"duration_ms,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

func (s *Store) InsertAgentToolInvocation(ctx context.Context, orgID, userID uuid.UUID, sessionID *string, tool string, argsJSON, resultJSON json.RawMessage, errText *string, durationMs *int) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO agent_tool_invocations (
			organization_id, user_id, session_id, tool, arguments_json, result_json, error_text, duration_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, orgID, userID, sessionID, tool, argsJSON, resultJSON, errText, durationMs)
	return err
}
