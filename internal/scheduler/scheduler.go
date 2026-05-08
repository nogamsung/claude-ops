package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
)

// TaskCanceller provides the ability to cancel a running task by ID.
type TaskCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

// Poller fetches new issues and enqueues them as tasks.
type Poller interface {
	Poll(ctx context.Context) error
}

// WorkerRunner executes a single queued task.
type WorkerRunner interface {
	RunTask(ctx context.Context, task *domain.Task) error
}

// BudgetGate reports whether a new task may be dispatched at now and why not
// when refused. The empty BudgetReason means allowed.
type BudgetGate interface {
	SnapshotReason(ctx context.Context, now time.Time) (BudgetReason, error)
}

// Scheduler drives the tick loop and dispatches tasks within active windows.
type Scheduler struct {
	clock        Clock
	windows      []*domain.ActiveWindow
	taskRepo     domain.TaskRepository
	appStateRepo domain.AppStateRepository
	poller       Poller
	worker       WorkerRunner
	budgetGate   BudgetGate

	tickInterval time.Duration

	// workerID identifies this scheduler instance; written into tasks.worker_id
	// by ClaimNext so the periodic reclaimer can attribute orphan rows.
	workerID string

	// sem caps the number of in-flight worker goroutines. Buffer = N
	// (cfg.MaxParallelTasks); N=1 reproduces v1 serial behavior exactly.
	sem chan struct{}

	// cancelMap maps taskID -> context cancel fn for running tasks.
	// CancelTask uses it for explicit /tasks/{id}/stop signals; graceful
	// shutdown is fan-out via the parent ctx the worker derives from.
	mu        sync.Mutex
	cancelMap map[string]context.CancelFunc

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Config holds constructor parameters for the Scheduler.
type Config struct {
	Clock            Clock
	Windows          []*domain.ActiveWindow
	TaskRepo         domain.TaskRepository
	AppStateRepo     domain.AppStateRepository
	Poller           Poller
	Worker           WorkerRunner
	BudgetGate       BudgetGate
	TickInterval     time.Duration
	WorkerID         string // opaque instance token (e.g. "host-pid"); empty disables ClaimNext attribution
	MaxParallelTasks int    // 1 = v1 byte-for-byte. Capped at 10 by config validation.
}

// New creates a new Scheduler.
func New(cfg Config) *Scheduler {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 30 * time.Second
	}
	if cfg.MaxParallelTasks < 1 {
		cfg.MaxParallelTasks = 1
	}
	return &Scheduler{
		clock:        cfg.Clock,
		windows:      cfg.Windows,
		taskRepo:     cfg.TaskRepo,
		appStateRepo: cfg.AppStateRepo,
		poller:       cfg.Poller,
		worker:       cfg.Worker,
		budgetGate:   cfg.BudgetGate,
		tickInterval: cfg.TickInterval,
		workerID:     cfg.WorkerID,
		sem:          make(chan struct{}, cfg.MaxParallelTasks),
		cancelMap:    make(map[string]context.CancelFunc),
		stopCh:       make(chan struct{}),
	}
}

// Start begins the tick loop; it blocks until ctx is cancelled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	slog.Info("scheduler: started", "tick_interval", s.tickInterval)

	for {
		select {
		case <-s.stopCh:
			slog.Info("scheduler: stop requested")
			return
		case <-ctx.Done():
			slog.Info("scheduler: context cancelled")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// Stop signals the scheduler to stop and waits for in-flight workers to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// CancelTask cancels a running task by its ID (implements TaskCanceller).
func (s *Scheduler) CancelTask(_ context.Context, taskID string) error {
	s.mu.Lock()
	fn, ok := s.cancelMap[taskID]
	s.mu.Unlock()

	if !ok {
		return domain.ErrNotFound
	}
	fn()
	return nil
}

// tick is called on every scheduler interval.
func (s *Scheduler) tick(ctx context.Context) {
	fullMode := s.isFullMode(ctx)

	if !AllowNow(s.clock.Now(), fullMode, s.windows) {
		slog.Debug("scheduler: outside active window, skipping tick")
		return
	}

	// Poll for new issues regardless of budget — polling is free and lets
	// queued items accumulate for when the gate reopens.
	if s.poller != nil {
		if err := s.poller.Poll(ctx); err != nil {
			slog.Error("scheduler: poller error", "err", err)
		}
	}

	// Budget gate (daily/weekly task cap + observed CLI rate-limit block).
	// Always enforced — even in full mode we refuse to spawn past these caps.
	if s.budgetGate != nil {
		reason, err := s.budgetGate.SnapshotReason(ctx, s.clock.Now())
		if err != nil {
			slog.Warn("scheduler: budget snapshot error", "err", err)
		} else if reason != BudgetReasonAllowed {
			slog.Debug("scheduler: budget gate blocks dispatch", "reason", string(reason))
			return
		}
	}

	// Fill every empty slot in this tick — relevant when MaxParallelTasks > 1
	// or when a slot just freed up. Each iteration first reserves a slot
	// (non-blocking), then asks the repository to atomically claim a task
	// whose repo has no in-flight work; loop exits on full slots or empty queue.
	for {
		select {
		case s.sem <- struct{}{}:
			dispatched := s.dispatchOne(ctx)
			if !dispatched {
				<-s.sem
				return
			}
		default:
			slog.Debug("scheduler: all worker slots busy")
			return
		}
	}
}

// dispatchOne atomically claims one task via ClaimNext (FOR UPDATE SKIP LOCKED
// + per-repo NOT EXISTS) and spawns a worker for it. Caller must already hold
// a slot in s.sem. Returns false when no eligible row was claimed (queue
// empty or every queued repo already has a running task).
func (s *Scheduler) dispatchOne(ctx context.Context) bool {
	task, err := s.taskRepo.ClaimNext(ctx, s.workerID)
	if err != nil {
		slog.Error("scheduler: claim next task", "err", err)
		return false
	}
	if task == nil {
		return false
	}

	taskCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.cancelMap[task.ID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.cancelMap, task.ID)
			s.mu.Unlock()
		}()

		slog.Info("scheduler: dispatching task",
			"task_id", task.ID, "repo", task.RepoFullName, "issue", task.IssueNumber, "worker_id", s.workerID)
		if err := s.worker.RunTask(taskCtx, task); err != nil {
			slog.Error("scheduler: worker error", "task_id", task.ID, "err", err)
		}
	}()

	return true
}

// isFullMode reads the full_mode app state from the database.
func (s *Scheduler) isFullMode(ctx context.Context) bool {
	if s.appStateRepo == nil {
		return false
	}
	state, err := s.appStateRepo.Get(ctx, "full_mode")
	if err != nil {
		return false
	}
	// Quick check: if the JSON contains "true" assume enabled.
	return len(state.ValueJSON) > 0 && containsTrue(state.ValueJSON)
}

func containsTrue(s string) bool {
	for i := 0; i < len(s)-3; i++ {
		if s[i:i+4] == "true" {
			return true
		}
	}
	return false
}
