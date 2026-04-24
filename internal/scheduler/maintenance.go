package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// MaintenanceEnqueuer enqueues a maintenance task by config.
type MaintenanceEnqueuer interface {
	EnqueueMaintenance(ctx context.Context, mt config.MaintenanceTaskConfig) (*domain.Task, error)
}

// MaintenanceScheduler registers cron jobs for each MaintenanceTaskConfig and
// runs them inside the same active-window constraint as the regular scheduler.
type MaintenanceScheduler struct {
	tasks    []config.MaintenanceTaskConfig
	windows  []*domain.ActiveWindow
	enqueuer MaintenanceEnqueuer
	clock    Clock
	cron     *cron.Cron
}

// MaintenanceSchedulerConfig holds constructor parameters.
type MaintenanceSchedulerConfig struct {
	Tasks    []config.MaintenanceTaskConfig
	Windows  []*domain.ActiveWindow
	Enqueuer MaintenanceEnqueuer
	Clock    Clock
}

// NewMaintenanceScheduler creates a MaintenanceScheduler. A nil Clock falls
// back to RealClock.
func NewMaintenanceScheduler(cfg MaintenanceSchedulerConfig) *MaintenanceScheduler {
	clk := cfg.Clock
	if clk == nil {
		clk = RealClock{}
	}
	return &MaintenanceScheduler{
		tasks:    cfg.Tasks,
		windows:  cfg.Windows,
		enqueuer: cfg.Enqueuer,
		clock:    clk,
		// Use seconds=false (standard 5-field cron spec: min hour dom month dow).
		cron: cron.New(),
	}
}

// Start registers all cron jobs and starts the cron runner. It blocks until ctx
// is cancelled.  Each cron job fires, checks the active window, and enqueues the
// maintenance task if the window is open.
func (ms *MaintenanceScheduler) Start(ctx context.Context) {
	if len(ms.tasks) == 0 {
		slog.Info("maintenance-scheduler: no maintenance tasks configured, exiting")
		return
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	for _, mt := range ms.tasks {
		mt := mt // capture for closure
		schedule, err := parser.Parse(mt.Cron)
		if err != nil {
			// Already validated at startup; log and skip defensively.
			slog.Error("maintenance-scheduler: invalid cron spec (skipping)", "name", mt.Name, "spec", mt.Cron, "err", err)
			continue
		}
		ms.cron.Schedule(schedule, cron.FuncJob(func() {
			ms.fire(ctx, mt)
		}))
		slog.Info("maintenance-scheduler: registered cron job", "name", mt.Name, "cron", mt.Cron)
	}

	ms.cron.Start()
	slog.Info("maintenance-scheduler: started")

	<-ctx.Done()

	// Graceful stop: wait for in-flight jobs to finish.
	stopCtx := ms.cron.Stop()
	<-stopCtx.Done()
	slog.Info("maintenance-scheduler: stopped")
}

// FireForTest exposes fire for white-box testing so tests can exercise the
// window-check and enqueue paths without waiting for a real cron tick.
func (ms *MaintenanceScheduler) FireForTest(ctx context.Context, mt config.MaintenanceTaskConfig) {
	ms.fire(ctx, mt)
}

// fire is the cron job handler for a single MaintenanceTaskConfig.
func (ms *MaintenanceScheduler) fire(ctx context.Context, mt config.MaintenanceTaskConfig) {
	now := ms.clock.Now()

	// Active window check — maintenance tasks must also respect the window.
	// full mode does NOT bypass this gate for maintenance (per spec).
	if !AllowNow(now, false, ms.windows) {
		slog.Info("maintenance-scheduler: outside active window, skipping", "name", mt.Name, "time", now.Format(time.RFC3339))
		return
	}

	task, err := ms.enqueuer.EnqueueMaintenance(ctx, mt)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		slog.Warn("maintenance-scheduler: enqueue skipped", "name", mt.Name, "reason", err.Error())
		return
	}
	slog.Info("maintenance-scheduler: task enqueued", "name", mt.Name, "task_id", task.ID, "repo", task.RepoFullName)
}

