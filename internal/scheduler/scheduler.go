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

// Scheduler drives the tick loop and dispatches tasks within active windows.
type Scheduler struct {
	clock        Clock
	windows      []*domain.ActiveWindow
	taskRepo     domain.TaskRepository
	appStateRepo domain.AppStateRepository
	poller       Poller
	worker       WorkerRunner

	tickInterval time.Duration

	// semaphore limits concurrent tasks to 1 (v1 serial execution).
	sem chan struct{}

	// cancelMap maps taskID -> context cancel fn for running tasks.
	mu        sync.Mutex
	cancelMap map[string]context.CancelFunc

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Config holds constructor parameters for the Scheduler.
type Config struct {
	Clock        Clock
	Windows      []*domain.ActiveWindow
	TaskRepo     domain.TaskRepository
	AppStateRepo domain.AppStateRepository
	Poller       Poller
	Worker       WorkerRunner
	TickInterval time.Duration
}

// New creates a new Scheduler.
func New(cfg Config) *Scheduler {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 30 * time.Second
	}
	return &Scheduler{
		clock:        cfg.Clock,
		windows:      cfg.Windows,
		taskRepo:     cfg.TaskRepo,
		appStateRepo: cfg.AppStateRepo,
		poller:       cfg.Poller,
		worker:       cfg.Worker,
		tickInterval: cfg.TickInterval,
		sem:          make(chan struct{}, 1),
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

	// Poll for new issues.
	if s.poller != nil {
		if err := s.poller.Poll(ctx); err != nil {
			slog.Error("scheduler: poller error", "err", err)
		}
	}

	// Dispatch a queued task if the semaphore is free.
	select {
	case s.sem <- struct{}{}:
		// Acquired semaphore — try to dispatch.
		dispatched := s.dispatch(ctx)
		if !dispatched {
			<-s.sem // Release immediately if nothing to run.
		}
	default:
		slog.Debug("scheduler: worker slot busy, skipping dispatch")
	}
}

// dispatch picks the oldest queued task and starts the worker.
// Returns true if a task was dispatched.
func (s *Scheduler) dispatch(ctx context.Context) bool {
	status := domain.TaskStatusQueued
	tasks, err := s.taskRepo.List(ctx, domain.TaskFilter{
		Status: &status,
		Limit:  1,
	})
	if err != nil {
		slog.Error("scheduler: list queued tasks", "err", err)
		return false
	}
	if len(tasks) == 0 {
		return false
	}

	task := tasks[0]
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

		slog.Info("scheduler: dispatching task", "task_id", task.ID, "repo", task.RepoFullName, "issue", task.IssueNumber)
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
