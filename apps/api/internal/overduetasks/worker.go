package overduetasks

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/store"
)

// Worker periodically moves past-due tasks into the board's "Overdue" column and notifies assignees.
type Worker struct {
	Store *store.Store
	Bus   *events.Bus
}

// Start runs until ctx is cancelled. The first sweep runs shortly after startup, then every minute.
func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.Store == nil {
		return
	}
	tick := time.NewTicker(1 * time.Minute)
	defer tick.Stop()

	w.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.sweep(ctx)
		}
	}
}

func (w *Worker) sweep(ctx context.Context) {
	rows, err := w.Store.ListOverduePromotionCandidates(ctx)
	if err != nil {
		slog.Warn("overdue tasks sweep list", "err", err)
		return
	}
	for _, row := range rows {
		w.promoteOne(ctx, row)
	}
}

func (w *Worker) promoteOne(ctx context.Context, row store.OverduePromotionRow) {
	maxPos, err := w.Store.MaxTaskPositionInColumn(ctx, row.OverdueColID)
	if err != nil {
		slog.Warn("overdue tasks max position", "err", err, "task_id", row.TaskID)
		return
	}
	nextPos := maxPos + 1
	t, err := w.Store.UpdateTask(ctx, row.SpaceID, row.TaskID, store.TaskPatch{
		ColumnID: &row.OverdueColID,
		Position: &nextPos,
	})
	if err != nil {
		slog.Warn("overdue tasks move", "err", err, "task_id", row.TaskID)
		return
	}

	if w.Bus != nil {
		payload, err := events.Marshal(events.TaskUpdated, row.OrgID, &row.SpaceID, t)
		if err == nil {
			_ = w.Bus.Publish(context.Background(), row.OrgID, payload)
		}
	}

	if row.AssigneeID == nil || *row.AssigneeID == uuid.Nil {
		return
	}

	payload := map[string]any{
		"organization_id": row.OrgID.String(),
		"space_id":        row.SpaceID.String(),
		"board_id":        row.BoardID.String(),
		"task_id":         row.TaskID.String(),
		"title":           row.Title,
		"due_at":          row.DueAt.UTC().Format(time.RFC3339),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	n, err := w.Store.CreateNotification(ctx, row.OrgID, *row.AssigneeID, "task.overdue", b)
	if err != nil {
		slog.Warn("overdue task notification", "err", err, "task_id", row.TaskID)
		return
	}

	if w.Bus != nil {
		pid := row.SpaceID
		payloadN, err := events.Marshal(events.NotificationCreated, row.OrgID, &pid, map[string]any{
			"user_id":      row.AssigneeID.String(),
			"notification": n,
		})
		if err == nil {
			_ = w.Bus.Publish(context.Background(), row.OrgID, payloadN)
		}
	}
}
