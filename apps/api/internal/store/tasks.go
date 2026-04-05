package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Task struct {
	ID                      uuid.UUID  `json:"id"`
	ProjectID               uuid.UUID  `json:"space_id"`
	BoardID                 uuid.UUID  `json:"board_id"`
	ColumnID                uuid.UUID  `json:"column_id"`
	Title                   string     `json:"title"`
	Description             string     `json:"description"`
	AssigneeUserID          *uuid.UUID `json:"assignee_user_id,omitempty"`
	DueAt                   *time.Time `json:"due_at,omitempty"`
	DeliverableRequired     bool       `json:"deliverable_required"`
	DeliverableInstructions string     `json:"deliverable_instructions"`
	Position                int        `json:"position"`
	Version                 int        `json:"version"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func (s *Store) ListTasksByProject(ctx context.Context, projectID uuid.UUID) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, board_id, column_id, title, description,
		       assignee_user_id, due_at, deliverable_required, deliverable_instructions,
		       position, version, created_at, updated_at
		FROM tasks WHERE space_id = $1
		ORDER BY column_id, position ASC, created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows pgx.Rows) ([]Task, error) {
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description,
			&t.AssigneeUserID, &t.DueAt, &t.DeliverableRequired, &t.DeliverableInstructions,
			&t.Position, &t.Version, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTask(ctx context.Context, projectID, taskID uuid.UUID) (Task, error) {
	var t Task
	err := s.Pool.QueryRow(ctx, `
		SELECT id, space_id, board_id, column_id, title, description,
		       assignee_user_id, due_at, deliverable_required, deliverable_instructions,
		       position, version, created_at, updated_at
		FROM tasks WHERE id = $1 AND space_id = $2
	`, taskID, projectID).Scan(
		&t.ID, &t.ProjectID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description,
		&t.AssigneeUserID, &t.DueAt, &t.DeliverableRequired, &t.DeliverableInstructions,
		&t.Position, &t.Version, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

type TaskPatch struct {
	Title                     *string
	Description               *string
	ColumnID                  *uuid.UUID
	Position                  *int
	AssigneeUserID            *uuid.UUID
	DueAt                     *time.Time
	ClearDueAt                *bool
	DeliverableRequired       *bool
	DeliverableInstructions   *string
}

func (s *Store) UpdateTask(ctx context.Context, projectID, taskID uuid.UUID, p TaskPatch) (Task, error) {
	t, err := s.GetTask(ctx, projectID, taskID)
	if err != nil {
		return Task{}, err
	}
	if p.Title != nil {
		t.Title = *p.Title
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.ColumnID != nil {
		t.ColumnID = *p.ColumnID
	}
	if p.Position != nil {
		t.Position = *p.Position
	}
	if p.AssigneeUserID != nil {
		t.AssigneeUserID = p.AssigneeUserID
	}
	if p.ClearDueAt != nil && *p.ClearDueAt {
		t.DueAt = nil
	} else if p.DueAt != nil {
		t.DueAt = p.DueAt
	}
	if p.DeliverableRequired != nil {
		t.DeliverableRequired = *p.DeliverableRequired
	}
	if p.DeliverableInstructions != nil {
		t.DeliverableInstructions = *p.DeliverableInstructions
	}

	err = s.Pool.QueryRow(ctx, `
		UPDATE tasks SET
			title = $3,
			description = $4,
			column_id = $5,
			position = $6,
			assignee_user_id = $7,
			due_at = $8,
			deliverable_required = $9,
			deliverable_instructions = $10,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND space_id = $2
		RETURNING id, space_id, board_id, column_id, title, description,
		          assignee_user_id, due_at, deliverable_required, deliverable_instructions,
		          position, version, created_at, updated_at
	`, taskID, projectID, t.Title, t.Description, t.ColumnID, t.Position, t.AssigneeUserID, t.DueAt,
		t.DeliverableRequired, t.DeliverableInstructions).Scan(
		&t.ID, &t.ProjectID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description,
		&t.AssigneeUserID, &t.DueAt, &t.DeliverableRequired, &t.DeliverableInstructions,
		&t.Position, &t.Version, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

// CreateTaskParams holds optional fields for new tasks.
type CreateTaskParams struct {
	Title                   string
	Description             string
	AssigneeUserID          *uuid.UUID
	DeliverableRequired     bool
	DeliverableInstructions string
}

func (s *Store) CreateTask(ctx context.Context, projectID, boardID, columnID uuid.UUID, position int, p CreateTaskParams) (Task, error) {
	var t Task
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO tasks (space_id, board_id, column_id, title, description, position,
		                   assignee_user_id, deliverable_required, deliverable_instructions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, space_id, board_id, column_id, title, description,
		          assignee_user_id, due_at, deliverable_required, deliverable_instructions,
		          position, version, created_at, updated_at
	`, projectID, boardID, columnID, p.Title, p.Description, position,
		p.AssigneeUserID, p.DeliverableRequired, p.DeliverableInstructions).Scan(
		&t.ID, &t.ProjectID, &t.BoardID, &t.ColumnID, &t.Title, &t.Description,
		&t.AssigneeUserID, &t.DueAt, &t.DeliverableRequired, &t.DeliverableInstructions,
		&t.Position, &t.Version, &t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

func (s *Store) DeleteTask(ctx context.Context, projectID, taskID uuid.UUID) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1 AND space_id = $2`, taskID, projectID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MaxTaskPositionInColumn(ctx context.Context, columnID uuid.UUID) (int, error) {
	var max int
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), -1)::int FROM tasks WHERE column_id = $1`, columnID).Scan(&max)
	return max, err
}

// GetBoardIDForColumnInProject resolves which board owns the column within a project.
func (s *Store) GetBoardIDForColumnInProject(ctx context.Context, projectID, columnID uuid.UUID) (uuid.UUID, error) {
	var bid uuid.UUID
	err := s.Pool.QueryRow(ctx, `
		SELECT b.id FROM board_columns c
		JOIN boards b ON b.id = c.board_id
		WHERE c.id = $1 AND b.space_id = $2
	`, columnID, projectID).Scan(&bid)
	return bid, err
}

// VerifyColumnBelongsToBoard checks column is under board and board belongs to project.
func (s *Store) VerifyColumnBelongsToProject(ctx context.Context, projectID, columnID uuid.UUID) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		SELECT 1 FROM board_columns c
		JOIN boards b ON b.id = c.board_id
		WHERE c.id = $1 AND b.space_id = $2
	`, columnID, projectID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MyAssignedTaskRow is a task assigned to the user with navigation fields.
type MyAssignedTaskRow struct {
	Task
	OrganizationID uuid.UUID `json:"organization_id"`
	SpaceName      string     `json:"space_name"`
}

// ListMyAssignedTasksInOrg returns tasks assigned to userID in spaces under orgID.
func (s *Store) ListMyAssignedTasksInOrg(ctx context.Context, orgID, userID uuid.UUID, limit int) ([]MyAssignedTaskRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.space_id, t.board_id, t.column_id, t.title, t.description,
		       t.assignee_user_id, t.due_at, t.deliverable_required, t.deliverable_instructions,
		       t.position, t.version, t.created_at, t.updated_at,
		       sp.organization_id, sp.name
		FROM tasks t
		JOIN spaces sp ON sp.id = t.space_id
		WHERE sp.organization_id = $1 AND t.assignee_user_id = $2
		ORDER BY t.updated_at DESC
		LIMIT $3
	`, orgID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MyAssignedTaskRow
	for rows.Next() {
		var r MyAssignedTaskRow
		if err := rows.Scan(
			&r.ID, &r.ProjectID, &r.BoardID, &r.ColumnID, &r.Title, &r.Description,
			&r.AssigneeUserID, &r.DueAt, &r.DeliverableRequired, &r.DeliverableInstructions,
			&r.Position, &r.Version, &r.CreatedAt, &r.UpdatedAt,
			&r.OrganizationID, &r.SpaceName,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
