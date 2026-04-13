package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type StaffMemoryEpisode struct {
	ID              uuid.UUID  `json:"id"`
	OrganizationID  uuid.UUID  `json:"organization_id"`
	ServiceAccountID uuid.UUID `json:"service_account_id"`
	SpaceID         *uuid.UUID `json:"space_id,omitempty"`
	ChatRoomID      *uuid.UUID `json:"chat_room_id,omitempty"`
	SourceMessageID *uuid.UUID `json:"source_message_id,omitempty"`
	ReplyMessageID  *uuid.UUID `json:"reply_message_id,omitempty"`
	Summary         string     `json:"summary"`
	Details         string     `json:"details"`
	Importance      float64    `json:"importance"`
	CreatedAt       time.Time  `json:"created_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
}

type StaffMemoryFact struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	ServiceAccountID uuid.UUID  `json:"service_account_id"`
	EpisodeID        *uuid.UUID `json:"episode_id,omitempty"`
	SourceMessageID  *uuid.UUID `json:"source_message_id,omitempty"`
	Statement        string     `json:"statement"`
	Confidence       float64    `json:"confidence"`
	CreatedAt        time.Time  `json:"created_at"`
	ValidUntil       *time.Time `json:"valid_until,omitempty"`
	InvalidatedAt    *time.Time `json:"invalidated_at,omitempty"`
}

type StaffMemoryProcedure struct {
	ID               uuid.UUID       `json:"id"`
	OrganizationID   uuid.UUID       `json:"organization_id"`
	ServiceAccountID uuid.UUID       `json:"service_account_id"`
	Name             string          `json:"name"`
	StepsJSON        json.RawMessage `json:"steps_json"`
	SuccessCount     int             `json:"success_count"`
	FailureCount     int             `json:"failure_count"`
	Version          int             `json:"version"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type CreateStaffMemoryEpisodeInput struct {
	OrganizationID   uuid.UUID
	ServiceAccountID uuid.UUID
	SpaceID          *uuid.UUID
	ChatRoomID       *uuid.UUID
	SourceMessageID  *uuid.UUID
	ReplyMessageID   *uuid.UUID
	Summary          string
	Details          string
	Importance       float64
}

func clampImportance(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (s *Store) TryClaimStaffMemoryRun(ctx context.Context, orgID, serviceAccountID, sourceMessageID, replyMessageID uuid.UUID) (bool, error) {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO staff_memory_runs (organization_id, service_account_id, source_message_id, reply_message_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (service_account_id, source_message_id) DO UPDATE
		SET reply_message_id = COALESCE(staff_memory_runs.reply_message_id, EXCLUDED.reply_message_id)
	`, orgID, serviceAccountID, sourceMessageID, replyMessageID)
	if err != nil {
		return false, err
	}
	var claimed bool
	err = s.Pool.QueryRow(ctx, `
		WITH claimed AS (
			UPDATE staff_memory_runs
			SET processed_at = now()
			WHERE organization_id = $1
			  AND service_account_id = $2
			  AND source_message_id = $3
			  AND processed_at IS NULL
			RETURNING 1
		)
		SELECT EXISTS(SELECT 1 FROM claimed)
	`, orgID, serviceAccountID, sourceMessageID).Scan(&claimed)
	return claimed, err
}

func (s *Store) CreateStaffMemoryEpisode(ctx context.Context, in CreateStaffMemoryEpisodeInput) (StaffMemoryEpisode, error) {
	var out StaffMemoryEpisode
	summary := strings.TrimSpace(in.Summary)
	if summary == "" {
		summary = "Conversation summary"
	}
	details := strings.TrimSpace(in.Details)
	importance := clampImportance(in.Importance)
	if importance == 0 {
		importance = 0.5
	}
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO staff_memory_episodes (
			organization_id, service_account_id, space_id, chat_room_id, source_message_id, reply_message_id,
			summary, details, importance
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, organization_id, service_account_id, space_id, chat_room_id, source_message_id, reply_message_id,
			summary, details, importance, created_at, deleted_at
	`, in.OrganizationID, in.ServiceAccountID, in.SpaceID, in.ChatRoomID, in.SourceMessageID, in.ReplyMessageID, summary, details, importance).Scan(
		&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.SpaceID, &out.ChatRoomID, &out.SourceMessageID, &out.ReplyMessageID,
		&out.Summary, &out.Details, &out.Importance, &out.CreatedAt, &out.DeletedAt,
	)
	return out, err
}

func (s *Store) UpsertStaffMemoryFact(ctx context.Context, orgID, serviceAccountID uuid.UUID, episodeID *uuid.UUID, sourceMessageID *uuid.UUID, statement string, confidence float64) (StaffMemoryFact, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return StaffMemoryFact{}, pgx.ErrNoRows
	}
	confidence = clampImportance(confidence)
	if confidence == 0 {
		confidence = 0.5
	}
	var existingID uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT id
		FROM staff_memory_facts
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND invalidated_at IS NULL
		  AND lower(btrim(statement)) = lower(btrim($3))
		ORDER BY created_at DESC
		LIMIT 1
	`, orgID, serviceAccountID, statement).Scan(&existingID)
	if err != nil && err != pgx.ErrNoRows {
		return StaffMemoryFact{}, err
	}
	var out StaffMemoryFact
	if err == nil {
		err = s.Pool.QueryRow(ctx, `
			UPDATE staff_memory_facts
			SET confidence = GREATEST(confidence, $2),
				episode_id = COALESCE($3, episode_id),
				source_message_id = COALESCE($4, source_message_id),
				valid_until = NULL,
				invalidated_at = NULL
			WHERE id = $1
			RETURNING id, organization_id, service_account_id, episode_id, source_message_id,
				statement, confidence, created_at, valid_until, invalidated_at
		`, existingID, confidence, episodeID, sourceMessageID).Scan(
			&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.EpisodeID, &out.SourceMessageID,
			&out.Statement, &out.Confidence, &out.CreatedAt, &out.ValidUntil, &out.InvalidatedAt,
		)
		return out, err
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO staff_memory_facts (
			organization_id, service_account_id, episode_id, source_message_id, statement, confidence
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, organization_id, service_account_id, episode_id, source_message_id,
			statement, confidence, created_at, valid_until, invalidated_at
	`, orgID, serviceAccountID, episodeID, sourceMessageID, statement, confidence).Scan(
		&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.EpisodeID, &out.SourceMessageID,
		&out.Statement, &out.Confidence, &out.CreatedAt, &out.ValidUntil, &out.InvalidatedAt,
	)
	return out, err
}

func (s *Store) UpsertStaffMemoryProcedure(ctx context.Context, orgID, serviceAccountID uuid.UUID, episodeID *uuid.UUID, name string, steps json.RawMessage) (StaffMemoryProcedure, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return StaffMemoryProcedure{}, pgx.ErrNoRows
	}
	if len(steps) == 0 {
		steps = json.RawMessage(`[]`)
	}
	var existingID uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT id
		FROM staff_memory_procedures
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND lower(btrim(name)) = lower(btrim($3))
		ORDER BY updated_at DESC
		LIMIT 1
	`, orgID, serviceAccountID, name).Scan(&existingID)
	if err != nil && err != pgx.ErrNoRows {
		return StaffMemoryProcedure{}, err
	}
	var out StaffMemoryProcedure
	if err == nil {
		err = s.Pool.QueryRow(ctx, `
			UPDATE staff_memory_procedures
			SET steps_json = $2,
				version = version + 1,
				source_episode_id = COALESCE($3, source_episode_id),
				updated_at = now()
			WHERE id = $1
			RETURNING id, organization_id, service_account_id, name, steps_json, success_count, failure_count, version, created_at, updated_at
		`, existingID, steps, episodeID).Scan(
			&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.Name, &out.StepsJSON, &out.SuccessCount, &out.FailureCount, &out.Version, &out.CreatedAt, &out.UpdatedAt,
		)
		return out, err
	}
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO staff_memory_procedures (
			organization_id, service_account_id, source_episode_id, name, steps_json
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, organization_id, service_account_id, name, steps_json, success_count, failure_count, version, created_at, updated_at
	`, orgID, serviceAccountID, episodeID, name, steps).Scan(
		&out.ID, &out.OrganizationID, &out.ServiceAccountID, &out.Name, &out.StepsJSON, &out.SuccessCount, &out.FailureCount, &out.Version, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (s *Store) ListStaffMemoryEpisodes(ctx context.Context, orgID, serviceAccountID uuid.UUID, limit int) ([]StaffMemoryEpisode, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, space_id, chat_room_id, source_message_id, reply_message_id,
			summary, details, importance, created_at, deleted_at
		FROM staff_memory_episodes
		WHERE organization_id = $1
		  AND service_account_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, orgID, serviceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaffMemoryEpisode
	for rows.Next() {
		var row StaffMemoryEpisode
		if err := rows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.SpaceID, &row.ChatRoomID, &row.SourceMessageID, &row.ReplyMessageID,
			&row.Summary, &row.Details, &row.Importance, &row.CreatedAt, &row.DeletedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListStaffMemoryFacts(ctx context.Context, orgID, serviceAccountID uuid.UUID, limit int) ([]StaffMemoryFact, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, episode_id, source_message_id, statement, confidence, created_at, valid_until, invalidated_at
		FROM staff_memory_facts
		WHERE organization_id = $1
		  AND service_account_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, orgID, serviceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaffMemoryFact
	for rows.Next() {
		var row StaffMemoryFact
		if err := rows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.EpisodeID, &row.SourceMessageID, &row.Statement, &row.Confidence, &row.CreatedAt, &row.ValidUntil, &row.InvalidatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListStaffMemoryProcedures(ctx context.Context, orgID, serviceAccountID uuid.UUID, limit int) ([]StaffMemoryProcedure, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, name, steps_json, success_count, failure_count, version, created_at, updated_at
		FROM staff_memory_procedures
		WHERE organization_id = $1
		  AND service_account_id = $2
		ORDER BY updated_at DESC
		LIMIT $3
	`, orgID, serviceAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaffMemoryProcedure
	for rows.Next() {
		var row StaffMemoryProcedure
		if err := rows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.Name, &row.StepsJSON, &row.SuccessCount, &row.FailureCount, &row.Version, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) SearchStaffMemoryForPrompt(ctx context.Context, orgID, serviceAccountID uuid.UUID, q string, episodeLimit, factLimit, procLimit int) (episodes []StaffMemoryEpisode, facts []StaffMemoryFact, procedures []StaffMemoryProcedure, err error) {
	if episodeLimit <= 0 {
		episodeLimit = 6
	}
	if factLimit <= 0 {
		factLimit = 8
	}
	if procLimit <= 0 {
		procLimit = 3
	}
	q = strings.TrimSpace(q)
	if q == "" {
		episodes, err = s.ListStaffMemoryEpisodes(ctx, orgID, serviceAccountID, episodeLimit)
		if err != nil {
			return nil, nil, nil, err
		}
		facts, err = s.ListStaffMemoryFacts(ctx, orgID, serviceAccountID, factLimit)
		if err != nil {
			return nil, nil, nil, err
		}
		procedures, err = s.ListStaffMemoryProcedures(ctx, orgID, serviceAccountID, procLimit)
		return episodes, facts, procedures, err
	}

	erows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, space_id, chat_room_id, source_message_id, reply_message_id,
			summary, details, importance, created_at, deleted_at
		FROM staff_memory_episodes
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND deleted_at IS NULL
		  AND to_tsvector('english', coalesce(summary, '') || ' ' || coalesce(details, ''))
			  @@ plainto_tsquery('english', $3)
		ORDER BY importance DESC, created_at DESC
		LIMIT $4
	`, orgID, serviceAccountID, q, episodeLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var row StaffMemoryEpisode
		if err := erows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.SpaceID, &row.ChatRoomID, &row.SourceMessageID, &row.ReplyMessageID,
			&row.Summary, &row.Details, &row.Importance, &row.CreatedAt, &row.DeletedAt,
		); err != nil {
			return nil, nil, nil, err
		}
		episodes = append(episodes, row)
	}
	if err := erows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(episodes) == 0 {
		episodes, err = s.ListStaffMemoryEpisodes(ctx, orgID, serviceAccountID, episodeLimit)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	frows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, episode_id, source_message_id, statement, confidence, created_at, valid_until, invalidated_at
		FROM staff_memory_facts
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND invalidated_at IS NULL
		  AND to_tsvector('english', coalesce(statement, '')) @@ plainto_tsquery('english', $3)
		ORDER BY confidence DESC, created_at DESC
		LIMIT $4
	`, orgID, serviceAccountID, q, factLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var row StaffMemoryFact
		if err := frows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.EpisodeID, &row.SourceMessageID, &row.Statement, &row.Confidence, &row.CreatedAt, &row.ValidUntil, &row.InvalidatedAt,
		); err != nil {
			return nil, nil, nil, err
		}
		facts = append(facts, row)
	}
	if err := frows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(facts) == 0 {
		facts, err = s.ListStaffMemoryFacts(ctx, orgID, serviceAccountID, factLimit)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	prows, err := s.Pool.Query(ctx, `
		SELECT id, organization_id, service_account_id, name, steps_json, success_count, failure_count, version, created_at, updated_at
		FROM staff_memory_procedures
		WHERE organization_id = $1
		  AND service_account_id = $2
		  AND to_tsvector('english', coalesce(name, '') || ' ' || coalesce(steps_json::text, '')) @@ plainto_tsquery('english', $3)
		ORDER BY updated_at DESC
		LIMIT $4
	`, orgID, serviceAccountID, q, procLimit)
	if err != nil {
		return nil, nil, nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var row StaffMemoryProcedure
		if err := prows.Scan(
			&row.ID, &row.OrganizationID, &row.ServiceAccountID, &row.Name, &row.StepsJSON, &row.SuccessCount, &row.FailureCount, &row.Version, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, nil, nil, err
		}
		procedures = append(procedures, row)
	}
	if err := prows.Err(); err != nil {
		return nil, nil, nil, err
	}
	if len(procedures) == 0 {
		procedures, err = s.ListStaffMemoryProcedures(ctx, orgID, serviceAccountID, procLimit)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return episodes, facts, procedures, nil
}

func (s *Store) DeleteStaffMemoryEpisode(ctx context.Context, orgID, serviceAccountID, episodeID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE staff_memory_episodes
		SET deleted_at = now()
		WHERE id = $1
		  AND organization_id = $2
		  AND service_account_id = $3
	`, episodeID, orgID, serviceAccountID)
	return err
}

func (s *Store) InvalidateStaffMemoryFact(ctx context.Context, orgID, serviceAccountID, factID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE staff_memory_facts
		SET invalidated_at = now(), valid_until = now()
		WHERE id = $1
		  AND organization_id = $2
		  AND service_account_id = $3
	`, factID, orgID, serviceAccountID)
	return err
}

func (s *Store) CascadeDeleteMemoryForChatMessage(ctx context.Context, messageID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_episodes
		SET deleted_at = now()
		WHERE source_message_id = $1 OR reply_message_id = $1
	`, messageID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_facts
		SET invalidated_at = now(), valid_until = now()
		WHERE source_message_id = $1
		  AND invalidated_at IS NULL
	`, messageID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_facts f
		SET invalidated_at = now(), valid_until = now()
		WHERE f.invalidated_at IS NULL
		  AND f.episode_id IN (
			SELECT e.id FROM staff_memory_episodes e WHERE e.deleted_at IS NOT NULL
		  )
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE service_account_profile_proposals
		SET source_message_id = NULL
		WHERE source_message_id = $1
	`, messageID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GroomStaffMemory(ctx context.Context, retention time.Duration) error {
	if retention <= 0 {
		retention = 90 * 24 * time.Hour
	}
	cutoff := time.Now().Add(-retention)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_episodes
		SET deleted_at = now()
		WHERE created_at < $1
		  AND deleted_at IS NULL
	`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_facts
		SET invalidated_at = now(), valid_until = now()
		WHERE invalidated_at IS NULL
		  AND created_at < $1
	`, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_memory_facts f
		SET invalidated_at = now(), valid_until = now()
		WHERE f.invalidated_at IS NULL
		  AND f.episode_id IN (
			SELECT e.id FROM staff_memory_episodes e WHERE e.deleted_at IS NOT NULL
		  )
	`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

