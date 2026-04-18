// Package usecase implements business logic orchestrating domain entities.
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gs97ahn/scheduled-dev-agent/internal/config"
	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// SchedulerCanceller can cancel a running task.
type SchedulerCanceller interface {
	CancelTask(ctx context.Context, taskID string) error
}

// WindowChecker reports whether the current time is within an active window.
type WindowChecker interface {
	IsAllowed(ctx context.Context) (bool, error)
}

// WindowGate checks whether the given time falls within an active window.
// fullMode=false means window-only check; fullMode=true always returns true.
type WindowGate interface { // ADDED
	AllowNow(now time.Time, fullMode bool) bool
}

// Clock abstracts time.Now() to allow deterministic testing.
type Clock interface { // ADDED
	Now() time.Time
}

// realClock returns real system time.
type realClock struct{} // ADDED

func (realClock) Now() time.Time { return time.Now() } // ADDED

// TaskUseCase implements task management business logic.
type TaskUseCase struct {
	taskRepo     domain.TaskRepository
	eventRepo    domain.TaskEventRepository
	appStateRepo domain.AppStateRepository
	canceller    SchedulerCanceller
	repos        []config.RepoConfig
	windowGate   WindowGate // ADDED
	clock        Clock      // ADDED
}

// EnqueueRequest is the input for manually triggering a task.
type EnqueueRequest struct {
	RepoFullName string
	IssueNumber  int
	IssueTitle   string
}

// TaskDetail is the usecase response for a single task with its recent events.
type TaskDetail struct {
	Task   *domain.Task
	Events []*domain.TaskEvent
}

// NewTaskUseCase creates a TaskUseCase.
func NewTaskUseCase(
	taskRepo domain.TaskRepository,
	eventRepo domain.TaskEventRepository,
	appStateRepo domain.AppStateRepository,
	canceller SchedulerCanceller,
	repos []config.RepoConfig,
	opts ...TaskUseCaseOption, // ADDED
) *TaskUseCase {
	uc := &TaskUseCase{
		taskRepo:     taskRepo,
		eventRepo:    eventRepo,
		appStateRepo: appStateRepo,
		canceller:    canceller,
		repos:        repos,
		clock:        realClock{}, // ADDED
	}
	for _, o := range opts { // ADDED
		o(uc) // ADDED
	} // ADDED
	return uc
}

// TaskUseCaseOption is a functional option for TaskUseCase.
type TaskUseCaseOption func(*TaskUseCase) // ADDED

// WithWindowGate injects a WindowGate into TaskUseCase.
func WithWindowGate(wg WindowGate) TaskUseCaseOption { // ADDED
	return func(uc *TaskUseCase) { uc.windowGate = wg } // ADDED
} // ADDED

// WithClock injects a Clock into TaskUseCase.
func WithClock(c Clock) TaskUseCaseOption { // ADDED
	return func(uc *TaskUseCase) { uc.clock = c } // ADDED
} // ADDED

// EnqueueFromIssue manually enqueues a task, respecting the active window unless full mode is on.
func (uc *TaskUseCase) EnqueueFromIssue(ctx context.Context, req EnqueueRequest) (*domain.Task, error) {
	// Validate repo is in allowlist.
	if !uc.isAllowedRepo(req.RepoFullName) {
		return nil, fmt.Errorf("repo %q: %w", req.RepoFullName, domain.ErrNotFound)
	}

	// Check window / full mode (US-5 §2): allow if inside window OR full mode ON.
	fullMode, err := uc.isFullMode(ctx)
	if err != nil {
		return nil, fmt.Errorf("check full mode: %w", err)
	}
	// MODIFIED: was `if !fullMode { return ErrOutsideActiveWindow }` — logic was inverted.
	// Now: block only when full mode is off AND the window gate rejects the current time.
	if !fullMode && uc.windowGate != nil && !uc.windowGate.AllowNow(uc.clock.Now(), false) { // MODIFIED
		return nil, domain.ErrOutsideActiveWindow // MODIFIED
	} // MODIFIED

	// Prevent duplicate.
	exists, err := uc.taskRepo.ExistsByRepoAndIssue(ctx, req.RepoFullName, req.IssueNumber)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if exists {
		return nil, domain.ErrAlreadyRunning
	}

	task := &domain.Task{
		ID:           uuid.New().String(),
		RepoFullName: req.RepoFullName,
		IssueNumber:  req.IssueNumber,
		IssueTitle:   req.IssueTitle,
		TaskType:     domain.TaskTypeFeature,
		Status:       domain.TaskStatusQueued,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err = uc.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

// GetTask returns a task with its recent events.
func (uc *TaskUseCase) GetTask(ctx context.Context, id string) (*TaskDetail, error) {
	task, err := uc.taskRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	events, err := uc.eventRepo.ListByTaskID(ctx, id, 50)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	return &TaskDetail{Task: task, Events: events}, nil
}

// ListTasks returns tasks with optional status filter.
func (uc *TaskUseCase) ListTasks(ctx context.Context, filter domain.TaskFilter) ([]*domain.Task, error) {
	tasks, err := uc.taskRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

// StopTask cancels a running task.
func (uc *TaskUseCase) StopTask(ctx context.Context, id string) error {
	task, err := uc.taskRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	if task.Status != domain.TaskStatusRunning && task.Status != domain.TaskStatusQueued {
		return domain.ErrTaskNotCancellable
	}

	if task.Status == domain.TaskStatusRunning {
		if err = uc.canceller.CancelTask(ctx, id); err != nil {
			return fmt.Errorf("cancel task: %w", err)
		}
	} else {
		// Queued — just mark as cancelled directly.
		now := time.Now()
		task.Status = domain.TaskStatusCancelled
		task.FinishedAt = &now
		if err = uc.taskRepo.Update(ctx, task); err != nil {
			return fmt.Errorf("update task: %w", err)
		}
	}
	return nil
}

func (uc *TaskUseCase) isAllowedRepo(name string) bool {
	for _, r := range uc.repos {
		if r.Name == name {
			return true
		}
	}
	return false
}

func (uc *TaskUseCase) isFullMode(ctx context.Context) (bool, error) {
	if uc.appStateRepo == nil {
		return false, nil
	}
	state, err := uc.appStateRepo.Get(ctx, "full_mode")
	if err != nil {
		if err == domain.ErrNotFound {
			return false, nil
		}
		return false, err
	}
	var v struct {
		Enabled bool `json:"enabled"`
	}
	if err = json.Unmarshal([]byte(state.ValueJSON), &v); err != nil {
		return false, nil
	}
	return v.Enabled, nil
}
