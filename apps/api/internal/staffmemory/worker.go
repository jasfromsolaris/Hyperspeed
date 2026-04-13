package staffmemory

import (
	"context"
	"log/slog"
	"time"

	"hyperspeed/api/internal/store"
)

type Worker struct {
	Store     *store.Store
	Interval  time.Duration
	Retention time.Duration
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.Store == nil {
		return
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	tick := time.NewTicker(interval)
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
	if err := w.Store.GroomStaffMemory(ctx, w.Retention); err != nil {
		slog.Warn("staff memory groom", "err", err)
	}
}

