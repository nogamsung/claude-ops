package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// fakes

// fakeWindowGate implements WindowGate with a fixed allow/deny response.
type fakeWindowGate struct { // ADDED
	allow bool // ADDED
} // ADDED

func (g *fakeWindowGate) AllowNow(_ time.Time, _ bool) bool { return g.allow } // ADDED

// fakeClock implements Clock with a fixed time.
type fakeClock struct { // ADDED
	t time.Time // ADDED
} // ADDED

func (c *fakeClock) Now() time.Time { return c.t } // ADDED

type fakeTaskRepo struct {
	tasks  []*domain.Task
	exists bool
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
	var out []*domain.Task
	for _, t := range r.tasks {
		if f.Status != nil && t.Status != *f.Status {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
func (r *fakeTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) { return nil, nil }
func (r *fakeTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return r.exists, nil
}

type fakeEventRepo struct{}

func (r *fakeEventRepo) Create(_ context.Context, _ *domain.TaskEvent) error { return nil }
func (r *fakeEventRepo) ListByTaskID(_ context.Context, _ string, _ int) ([]*domain.TaskEvent, error) {
	return nil, nil
}

type fakeAppStateRepo struct {
	states map[string]*domain.AppState
}

func (r *fakeAppStateRepo) Get(_ context.Context, key string) (*domain.AppState, error) {
	if s, ok := r.states[key]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}
func (r *fakeAppStateRepo) Set(_ context.Context, s *domain.AppState) error {
	if r.states == nil {
		r.states = make(map[string]*domain.AppState)
	}
	r.states[s.Key] = s
	return nil
}

type fakeCanceller struct {
	cancelled []string
}

func (c *fakeCanceller) CancelTask(_ context.Context, id string) error {
	c.cancelled = append(c.cancelled, id)
	return nil
}

// tests

// TestTaskUseCase_EnqueueFromIssue_InsideWindow_FullModeOff verifies C1 fix:
// when inside the active window and full mode is off, enqueue must succeed.
func TestTaskUseCase_EnqueueFromIssue_InsideWindow_FullModeOff(t *testing.T) { // ADDED
	taskRepo := &fakeTaskRepo{}
	appStateRepo := &fakeAppStateRepo{
		states: map[string]*domain.AppState{
			"full_mode": {Key: "full_mode", ValueJSON: `{"enabled":false}`, UpdatedAt: time.Now()},
		},
	}
	repos := []config.RepoConfig{{Name: "owner/repo"}}
	gate := &fakeWindowGate{allow: true} // window allows this time

	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, &fakeCanceller{}, repos,
		usecase.WithWindowGate(gate),
		usecase.WithClock(&fakeClock{t: time.Now()}),
	)

	task, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/repo",
		IssueNumber:  1,
		IssueTitle:   "Inside window",
	})
	if err != nil {
		t.Fatalf("expected success inside window, got: %v", err)
	}
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued, got %s", task.Status)
	}
}

// TestTaskUseCase_EnqueueFromIssue_OutsideWindow_FullModeOff verifies C1 fix:
// when outside the active window and full mode is off, must return ErrOutsideActiveWindow.
func TestTaskUseCase_EnqueueFromIssue_OutsideWindow_FullModeOff(t *testing.T) { // ADDED
	taskRepo := &fakeTaskRepo{}
	appStateRepo := &fakeAppStateRepo{
		states: map[string]*domain.AppState{
			"full_mode": {Key: "full_mode", ValueJSON: `{"enabled":false}`, UpdatedAt: time.Now()},
		},
	}
	repos := []config.RepoConfig{{Name: "owner/repo"}}
	gate := &fakeWindowGate{allow: false} // window denies this time

	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, &fakeCanceller{}, repos,
		usecase.WithWindowGate(gate),
		usecase.WithClock(&fakeClock{t: time.Now()}),
	)

	_, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/repo",
		IssueNumber:  1,
	})
	if err != domain.ErrOutsideActiveWindow {
		t.Errorf("expected ErrOutsideActiveWindow, got: %v", err)
	}
}

// TestTaskUseCase_EnqueueFromIssue_OutsideWindow_FullModeOn verifies C1 fix:
// when outside the active window but full mode is on, enqueue must succeed.
func TestTaskUseCase_EnqueueFromIssue_OutsideWindow_FullModeOn(t *testing.T) { // ADDED
	taskRepo := &fakeTaskRepo{}
	appStateRepo := &fakeAppStateRepo{
		states: map[string]*domain.AppState{
			"full_mode": {Key: "full_mode", ValueJSON: `{"enabled":true}`, UpdatedAt: time.Now()},
		},
	}
	repos := []config.RepoConfig{{Name: "owner/repo"}}
	gate := &fakeWindowGate{allow: false} // window denies, but full mode overrides

	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, &fakeCanceller{}, repos,
		usecase.WithWindowGate(gate),
		usecase.WithClock(&fakeClock{t: time.Now()}),
	)

	task, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/repo",
		IssueNumber:  42,
		IssueTitle:   "Full mode bypass",
	})
	if err != nil {
		t.Fatalf("expected full mode to bypass window gate, got: %v", err)
	}
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued, got %s", task.Status)
	}
}

// TestTaskUseCase_EnqueueFromIssue_FullModeOff_NoGate verifies that when no WindowGate
// is injected, full mode off still allows enqueueing (gate is optional).
func TestTaskUseCase_EnqueueFromIssue_FullModeOff_NoGate(t *testing.T) { // MODIFIED: replaces old test that tested inverted logic
	taskRepo := &fakeTaskRepo{}
	appStateRepo := &fakeAppStateRepo{
		states: map[string]*domain.AppState{
			"full_mode": {Key: "full_mode", ValueJSON: `{"enabled":false}`, UpdatedAt: time.Now()},
		},
	}
	repos := []config.RepoConfig{{Name: "owner/repo"}}

	// No WithWindowGate — nil gate means no window check, so enqueue proceeds.
	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, &fakeCanceller{}, repos)

	task, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/repo",
		IssueNumber:  1,
	})
	if err != nil {
		t.Fatalf("expected success when no gate injected, got: %v", err)
	}
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued, got %s", task.Status)
	}
}

func TestTaskUseCase_EnqueueFromIssue_FullModeOn(t *testing.T) {
	taskRepo := &fakeTaskRepo{}
	appStateRepo := &fakeAppStateRepo{
		states: map[string]*domain.AppState{
			"full_mode": {Key: "full_mode", ValueJSON: `{"enabled":true}`, UpdatedAt: time.Now()},
		},
	}
	repos := []config.RepoConfig{{Name: "owner/repo"}}

	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, appStateRepo, &fakeCanceller{}, repos)

	task, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/repo",
		IssueNumber:  42,
		IssueTitle:   "Test issue",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected queued, got %s", task.Status)
	}
}

func TestTaskUseCase_EnqueueFromIssue_UnknownRepo(t *testing.T) {
	uc := usecase.NewTaskUseCase(
		&fakeTaskRepo{},
		&fakeEventRepo{},
		&fakeAppStateRepo{},
		&fakeCanceller{},
		[]config.RepoConfig{{Name: "owner/other"}},
	)

	_, err := uc.EnqueueFromIssue(context.Background(), usecase.EnqueueRequest{
		RepoFullName: "owner/unknown",
		IssueNumber:  1,
	})
	if err == nil {
		t.Error("expected error for unknown repo")
	}
}

func TestTaskUseCase_StopTask_Queued(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{{ID: "t1", Status: domain.TaskStatusQueued}},
	}
	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, &fakeAppStateRepo{}, &fakeCanceller{}, nil)

	if err := uc.StopTask(context.Background(), "t1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taskRepo.tasks[0].Status != domain.TaskStatusCancelled {
		t.Errorf("expected cancelled, got %s", taskRepo.tasks[0].Status)
	}
}

func TestTaskUseCase_StopTask_Running(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{{ID: "t2", Status: domain.TaskStatusRunning}},
	}
	canceller := &fakeCanceller{}
	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, &fakeAppStateRepo{}, canceller, nil)

	if err := uc.StopTask(context.Background(), "t2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(canceller.cancelled) != 1 || canceller.cancelled[0] != "t2" {
		t.Errorf("expected canceller to be called with t2")
	}
}

func TestTaskUseCase_StopTask_Done(t *testing.T) {
	taskRepo := &fakeTaskRepo{
		tasks: []*domain.Task{{ID: "t3", Status: domain.TaskStatusDone}},
	}
	uc := usecase.NewTaskUseCase(taskRepo, &fakeEventRepo{}, &fakeAppStateRepo{}, &fakeCanceller{}, nil)

	err := uc.StopTask(context.Background(), "t3")
	if err != domain.ErrTaskNotCancellable {
		t.Errorf("expected ErrTaskNotCancellable, got: %v", err)
	}
}

func TestTaskUseCase_GetTask_NotFound(t *testing.T) {
	uc := usecase.NewTaskUseCase(&fakeTaskRepo{}, &fakeEventRepo{}, &fakeAppStateRepo{}, &fakeCanceller{}, nil)
	_, err := uc.GetTask(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}
