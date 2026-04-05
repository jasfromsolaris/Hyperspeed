package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OverduePromotionRow is a task that should move to the board's "Overdue" column.
// Automation matches columns by name (case-insensitive): overdue (destination), done (excluded).
type OverduePromotionRow struct {
	TaskID       uuid.UUID
	SpaceID      uuid.UUID
	BoardID      uuid.UUID
	OrgID        uuid.UUID
	OverdueColID uuid.UUID
	Title        string
	AssigneeID   *uuid.UUID
	DueAt        time.Time
}

// ListOverduePromotionCandidates returns tasks with due_at in the past that are not in Done or Overdue.
func (s *Store) ListOverduePromotionCandidates(ctx context.Context) ([]OverduePromotionRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.space_id, t.board_id, sp.organization_id, oc.id,
		       t.title, t.assignee_user_id, t.due_at
		FROM tasks t
		JOIN spaces sp ON sp.id = t.space_id
		JOIN LATERAL (
			SELECT c.id
			FROM board_columns c
			WHERE c.board_id = t.board_id AND lower(trim(c.name)) = 'overdue'
			LIMIT 1
		) oc ON true
		WHERE t.due_at IS NOT NULL
		  AND t.due_at < now()
		  AND t.column_id <> oc.id
		  AND NOT EXISTS (
			SELECT 1 FROM board_columns cur
			WHERE cur.id = t.column_id AND lower(trim(cur.name)) = 'done'
		  )
		ORDER BY t.due_at ASC, t.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OverduePromotionRow
	for rows.Next() {
		var r OverduePromotionRow
		if err := rows.Scan(
			&r.TaskID, &r.SpaceID, &r.BoardID, &r.OrgID, &r.OverdueColID,
			&r.Title, &r.AssigneeID, &r.DueAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
