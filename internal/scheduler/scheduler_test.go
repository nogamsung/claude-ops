package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
	"github.com/gs97ahn/scheduled-dev-agent/internal/scheduler"
)

// --- minimal fakes ---

type fakeTaskRepo struct {
	tasks  []*domain.Task
	called atomic.Int32
}

func (r *fakeTaskRepo) Create(_ context.Context, t *domain.Task) error {
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *fakeTaskRepo) GetByID(_ context.Context, id string) (*domain.Task, error) {
	for _, t := range r.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeTaskRepo) Update(_ context.Context, t *domain.Task) error {
	for i, existing := range r.tasks {
		if existing.ID == t.ID {
			r.tasks[i] = t
			return nil
		}
	}
	return nil
}
func (r *fakeTaskRepo) List(_ context.Context, f domain.TaskFilter) ([]*domain.Task, error) {
	r.called.Add(1)
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
func (r *fakeTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) {
	var out []*domain.Task
	for _, t := range r.tasks {
		if t.Status == domain.TaskStatusRunning {
			out = append(out, t)
		}
	}
	return out, nil
}
func (r *fakeTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return false, nil
}

type fakePoller struct {
	calls atomic.Int32
}

func (p *fakePoller) Poll(_ context.Context) error {
	p.calls.Add(1)
	return nil
}

type fakeWorker struct {
	calls atomic.Int32
}

func (w *fakeWorker) RunTask(_ context.Context, _ *domain.Task) error {
	w.calls.Add(1)
	return nil
}

// --- tests ---

func TestScheduler_OutsideWindow_NoCalls(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon", "tue", "wed", "thu", "fri"}, "09:00", "18:00", "Asia/Seoul")

	// Saturday 14:00 Seoul — outside window
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)}
	poller := &fakePoller{}
	worker := &fakeWorker{}
	repo := &fakeTaskRepo{tasks: []*domain.Task{{ID: "t1", Status: domain.TaskStatusQueued}}}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		Poller:       poller,
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	if poller.calls.Load() > 0 {
		t.Errorf("expected 0 poller calls outside window, got %d", poller.calls.Load())
	}
	if worker.calls.Load() > 0 {
		t.Errorf("expected 0 worker calls outside window, got %d (e2e: claude 0 calls)", worker.calls.Load())
	}
}

func TestScheduler_InsideWindow_Polls(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon"}, "09:00", "18:00", "Asia/Seoul")

	// Monday 10:00 Seoul — inside window
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 20, 10, 0, 0, 0, seoulLoc)}
	poller := &fakePoller{}
	worker := &fakeWorker{}
	repo := &fakeTaskRepo{}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		Poller:       poller,
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	if poller.calls.Load() == 0 {
		t.Error("expected poller to be called inside window")
	}
}

func TestScheduler_CancelTask_NotFound(t *testing.T) {
	sched := scheduler.New(scheduler.Config{
		TaskRepo:     &fakeTaskRepo{},
		TickInterval: time.Minute,
	})
	err := sched.CancelTask(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected ErrNotFound for nonexistent task")
	}
}

func TestScheduler_FullModeBypassesWindow(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon", "tue", "wed", "thu", "fri"}, "09:00", "18:00", "Asia/Seoul")

	// Saturday — outside window but full mode on
	fakeClock := &scheduler.FakeClock{T: time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)}
	poller := &fakePoller{}
	worker := &fakeWorker{}
	repo := &fakeTaskRepo{}
	appState := &fakeAppStateRepo{valueJSON: `{"enabled":true}`}

	sched := scheduler.New(scheduler.Config{
		Clock:        fakeClock,
		Windows:      []*domain.ActiveWindow{w},
		TaskRepo:     repo,
		AppStateRepo: appState,
		Poller:       poller,
		Worker:       worker,
		TickInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	sched.Start(ctx)

	if poller.calls.Load() == 0 {
		t.Error("expected poller to be called in full mode even outside window")
	}
}

type fakeAppStateRepo struct {
	valueJSON string
}

func (r *fakeAppStateRepo) Get(_ context.Context, _ string) (*domain.AppState, error) {
	return &domain.AppState{Key: "full_mode", ValueJSON: r.valueJSON}, nil
}
func (r *fakeAppStateRepo) Set(_ context.Context, _ *domain.AppState) error { return nil }
