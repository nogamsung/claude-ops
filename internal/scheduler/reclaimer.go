package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// Reclaimer is the periodic safety net that flips long-stalled "running"
// tasks to "orphaned" once their claim is older than `lease`. It exists so a
// crashed worker or hung claude process doesn't permanently strand a row.
//
// Default lease=60m, interval=10m → at most one reclaim cycle of staleness.
type Reclaimer struct {
	repo     domain.TaskRepository
	clock    Clock
	interval time.Duration
	lease    time.Duration
}

// NewReclaimer constructs a Reclaimer.
func NewReclaimer(repo domain.TaskRepository, clock Clock, interval, lease time.Duration) *Reclaimer {
	if clock == nil {
		clock = RealClock{}
	}
	return &Reclaimer{repo: repo, clock: clock, interval: interval, lease: lease}
}

// Start blocks until ctx is cancelled, calling ReclaimStale every interval.
// Errors are logged and the loop continues — losing one cycle is fine, the
// next one will retry.
func (r *Reclaimer) Start(ctx context.Context) {
	if r.repo == nil || r.interval <= 0 || r.lease <= 0 {
		slog.Warn("reclaimer: not configured (repo/interval/lease missing) — disabled")
		return
	}
	slog.Info("reclaimer: started", "interval", r.interval, "lease", r.lease)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reclaimer: stopping")
			return
		case <-ticker.C:
			cutoff := r.clock.Now().Add(-r.lease)
			n, err := r.repo.ReclaimStale(ctx, cutoff)
			if err != nil {
				slog.Error("reclaimer: stale reclaim failed", "err", err)
				continue
			}
			if n > 0 {
				slog.Warn("reclaimer: orphaned stale tasks", "count", n, "cutoff", cutoff)
			}
		}
	}
}
