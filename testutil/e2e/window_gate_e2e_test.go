// Package e2e contains end-to-end regression tests.
// Key invariant: when the clock is outside the active window,
// the scheduler must NOT call the claude runner (0 invocations).
package e2e_test

import (
	"context"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/claude"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

// countingWorker counts how many times RunTask is called (proxy for claude invocations).
type countingWorker struct {
	count atomic.Int64
}

func (w *countingWorker) RunTask(_ context.Context, _ *domain.Task) error {
	w.count.Add(1)
	return nil
}

// noopPoller does nothing.
type noopPoller struct{}

func (p *noopPoller) Poll(_ context.Context) error { return nil }

// preloadedTaskRepo always returns 1 queued task.
type preloadedTaskRepo struct {
	tasks []*domain.Task
}

func (r *preloadedTaskRepo) Create(_ context.Context, t *domain.Task) error {
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *preloadedTaskRepo) GetByID(_ context.Context, id string) (*domain.Task, error) {
	for _, t := range r.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *preloadedTaskRepo) Update(_ context.Context, t *domain.Task) error {
	for i, existing := range r.tasks {
		if existing.ID == t.ID {
			r.tasks[i] = t
		}
	}
	return nil
}
func (r *preloadedTaskRepo) List(_ context.Context, f domain.TaskFilter) ([]*domain.Task, error) {
	var out []*domain.Task
	for _, t := range r.tasks {
		if f.Status != nil && t.Status != *f.Status {
			continue
		}
		out = append(out, t)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}
func (r *preloadedTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) { return nil, nil }
func (r *preloadedTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return false, nil
}

// TestE2E_OutsideWindow_ZeroClaudeInvocations verifies that when the clock
// is outside the active time window and full-mode is off, the worker
// (and therefore the claude CLI) is never called.
func TestE2E_OutsideWindow_ZeroClaudeInvocations(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := &domain.ActiveWindow{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Start: "09:00", End: "18:00", TZ: "Asia/Seoul"}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}

	// Saturday 14:00 Seoul — outside window
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)}

	worker := &countingWorker{}
	repo := &preloadedTaskRepo{
		tasks: []*domain.Task{
			{ID: "task-e2e-1", Status: domain.TaskStatusQueued, CreatedAt: time.Now()},
		},
	}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		Poller:       &noopPoller{},
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	// INVARIANT: claude runner must be called exactly 0 times outside the window.
	if n := worker.count.Load(); n != 0 {
		t.Errorf("E2E FAIL: expected 0 claude invocations outside window, got %d", n)
	}
}

// TestE2E_InsideWindow_TaskDispatched verifies that inside the window,
// a queued task is dispatched (worker called at least once).
func TestE2E_InsideWindow_TaskDispatched(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := &domain.ActiveWindow{Days: []string{"mon"}, Start: "09:00", End: "18:00", TZ: "Asia/Seoul"}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}

	// Monday 10:00 Seoul — inside window
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 20, 10, 0, 0, 0, seoulLoc)}

	worker := &countingWorker{}
	repo := &preloadedTaskRepo{
		tasks: []*domain.Task{
			{ID: "task-e2e-2", Status: domain.TaskStatusQueued, CreatedAt: time.Now()},
		},
	}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		Poller:       &noopPoller{},
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	if n := worker.count.Load(); n < 1 {
		t.Errorf("E2E FAIL: expected at least 1 task dispatch inside window, got %d", n)
	}
}

// TestE2E_FullMode_BypassesWindow verifies full-mode allows tasks outside the window.
func TestE2E_FullMode_BypassesWindow(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := &domain.ActiveWindow{Days: []string{"mon", "tue", "wed", "thu", "fri"}, Start: "09:00", End: "18:00", TZ: "Asia/Seoul"}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}

	// Saturday — outside window
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)}

	worker := &countingWorker{}
	repo := &preloadedTaskRepo{
		tasks: []*domain.Task{
			{ID: "task-e2e-3", Status: domain.TaskStatusQueued, CreatedAt: time.Now()},
		},
	}

	// Full mode enabled via AppState.
	appState := &fakeAppStateRepoE2E{valueJSON: `{"enabled":true}`}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		AppStateRepo: appState,
		Poller:       &noopPoller{},
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	if n := worker.count.Load(); n < 1 {
		t.Errorf("E2E FAIL: expected full mode to bypass window gate, got %d dispatches", n)
	}
}

type fakeAppStateRepoE2E struct {
	valueJSON string
}

func (r *fakeAppStateRepoE2E) Get(_ context.Context, _ string) (*domain.AppState, error) {
	return &domain.AppState{Key: "full_mode", ValueJSON: r.valueJSON}, nil
}
func (r *fakeAppStateRepoE2E) Set(_ context.Context, _ *domain.AppState) error { return nil }

// --- C2 Regression: Runner double-gate uses injected clock, not time.Now() ---

// funcGate adapts a function to claude.WindowGate for testing.
type funcGate struct { // ADDED
	fn func(now time.Time, fullMode bool) bool // ADDED
} // ADDED

func (g *funcGate) AllowNow(now time.Time, fullMode bool) bool { return g.fn(now, fullMode) } // ADDED

// TestE2E_RunnerDoubleGate_OutsideWindow_ZeroInvocations verifies that
// when the FakeClock is set to a time outside the active window and the
// same clock is shared with the Runner, Run() returns ErrOutsideActiveWindow
// without executing any process (0 real exec.Cmd spawns).
func TestE2E_RunnerDoubleGate_OutsideWindow_ZeroInvocations(t *testing.T) { // ADDED
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := &domain.ActiveWindow{
		Days:  []string{"mon", "tue", "wed", "thu", "fri"},
		Start: "09:00",
		End:   "18:00",
		TZ:    "Asia/Seoul",
	}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}

	// Saturday 14:00 Seoul — outside window
	outsideTime := time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)
	fakeClock := &scheduler.FakeClock{T: outsideTime}

	var spawnCount atomic.Int64
	fakeExec := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		spawnCount.Add(1)
		return exec.CommandContext(ctx, "true") // would succeed, but should never be reached
	}

	runner := claude.NewRunnerWithExecAndClock(fakeExec, fakeClock)

	gate := &funcGate{fn: func(now time.Time, fullMode bool) bool {
		if fullMode {
			return true
		}
		return w.Contains(now)
	}}

	_, err := runner.Run(context.Background(), claude.RunInput{
		Prompt:     "test",
		Worktree:   t.TempDir(),
		FullMode:   false,
		WindowGate: gate,
	})

	if err != domain.ErrOutsideActiveWindow {
		t.Errorf("C2 regression: expected ErrOutsideActiveWindow from runner double-gate, got: %v", err)
	}
	if n := spawnCount.Load(); n != 0 {
		t.Errorf("C2 regression: expected 0 process spawns outside window, got %d", n)
	}
}
