package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

// --- fakes ---

type fakeMaintenanceTaskRepo struct {
	tasks []*domain.Task
}

func (r *fakeMaintenanceTaskRepo) Create(_ context.Context, t *domain.Task) error {
	r.tasks = append(r.tasks, t)
	return nil
}
func (r *fakeMaintenanceTaskRepo) GetByID(_ context.Context, _ string) (*domain.Task, error) {
	return nil, domain.ErrNotFound
}
func (r *fakeMaintenanceTaskRepo) Update(_ context.Context, _ *domain.Task) error { return nil }
func (r *fakeMaintenanceTaskRepo) List(_ context.Context, _ domain.TaskFilter) ([]*domain.Task, error) {
	return r.tasks, nil
}
func (r *fakeMaintenanceTaskRepo) GetRunning(_ context.Context) ([]*domain.Task, error) {
	return nil, nil
}
func (r *fakeMaintenanceTaskRepo) ExistsByRepoAndIssue(_ context.Context, _ string, _ int) (bool, error) {
	return false, nil
}

type fakeMaintenanceAppStateRepo struct {
	states map[string]*domain.AppState
}

func (r *fakeMaintenanceAppStateRepo) Get(_ context.Context, key string) (*domain.AppState, error) {
	if r.states == nil {
		return nil, domain.ErrNotFound
	}
	s, ok := r.states[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s, nil
}

func (r *fakeMaintenanceAppStateRepo) Set(_ context.Context, s *domain.AppState) error {
	if r.states == nil {
		r.states = make(map[string]*domain.AppState)
	}
	r.states[s.Key] = s
	return nil
}

// fakeBudgetEnforcer allows controlling the budget gate response.
type fakeBudgetEnforcer struct {
	reason scheduler.BudgetReason
	err    error
}

func (f *fakeBudgetEnforcer) CheckAndIncrementReason(_ context.Context, _ time.Time) (scheduler.BudgetReason, error) {
	return f.reason, f.err
}

func makeMaintenanceUC(
	taskRepo domain.TaskRepository,
	stateRepo domain.AppStateRepository,
	enforcer usecase.MaintenanceBudgetEnforcer,
) *usecase.MaintenanceUseCase {
	limits := scheduler.BudgetLimits{
		ResetTZ:      time.UTC,
		WeekStartsOn: time.Monday,
	}
	return usecase.NewMaintenanceUseCase(taskRepo, stateRepo, enforcer, limits)
}

func sampleMT(name string) config.MaintenanceTaskConfig {
	return config.MaintenanceTaskConfig{
		Name:           name,
		Cron:           "0 2 * * *",
		Repo:           "owner/repo",
		PromptTemplate: "maintenance/dep-update.tmpl",
		BudgetSubCap: config.SubCapConfig{
			Daily:  1,
			Weekly: 3,
		},
	}
}

// TestEnqueueMaintenance_Success verifies that a maintenance task is created
// and has the correct source and maintenance_name fields.
func TestEnqueueMaintenance_Success(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}
	budget := &fakeBudgetEnforcer{reason: scheduler.BudgetReasonAllowed}

	uc := makeMaintenanceUC(taskRepo, stateRepo, budget)
	mt := sampleMT("daily-dep-update")

	task, err := uc.EnqueueMaintenance(context.Background(), mt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task == nil {
		t.Fatal("expected task, got nil")
	}
	if task.Source != domain.TaskSourceMaintenance {
		t.Errorf("expected source=%q, got %q", domain.TaskSourceMaintenance, task.Source)
	}
	if task.MaintenanceName != "daily-dep-update" {
		t.Errorf("expected maintenance_name=%q, got %q", "daily-dep-update", task.MaintenanceName)
	}
	if task.Status != domain.TaskStatusQueued {
		t.Errorf("expected status=queued, got %q", task.Status)
	}
	if len(taskRepo.tasks) != 1 {
		t.Errorf("expected 1 task in repo, got %d", len(taskRepo.tasks))
	}
}

// TestEnqueueMaintenance_GlobalBudgetBlocked verifies that a blocked global budget
// prevents the maintenance task from being enqueued.
func TestEnqueueMaintenance_GlobalBudgetBlocked(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}
	budget := &fakeBudgetEnforcer{reason: scheduler.BudgetReasonDailyCap}

	uc := makeMaintenanceUC(taskRepo, stateRepo, budget)
	mt := sampleMT("daily-dep-update")

	task, err := uc.EnqueueMaintenance(context.Background(), mt)
	if err == nil {
		t.Fatal("expected error when global budget is blocked, got nil")
	}
	if task != nil {
		t.Errorf("expected nil task, got %+v", task)
	}
	if len(taskRepo.tasks) != 0 {
		t.Errorf("expected 0 tasks in repo, got %d", len(taskRepo.tasks))
	}
}

// TestEnqueueMaintenance_SubCapDailyReached verifies that exceeding the daily
// sub-cap prevents enqueuing, even when the global budget is open.
func TestEnqueueMaintenance_SubCapDailyReached(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}
	budget := &fakeBudgetEnforcer{reason: scheduler.BudgetReasonAllowed}

	uc := makeMaintenanceUC(taskRepo, stateRepo, budget)
	mt := sampleMT("daily-dep-update") // daily cap = 1

	// First call should succeed.
	if _, err := uc.EnqueueMaintenance(context.Background(), mt); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}

	// Second call on the same day should be rejected by sub-cap.
	// Reuse global budget with allowed (sub-cap is now at 1/1).
	_, err := uc.EnqueueMaintenance(context.Background(), mt)
	if err == nil {
		t.Fatal("expected sub-cap error on second call, got nil")
	}
	if len(taskRepo.tasks) != 1 {
		t.Errorf("expected 1 task in repo, got %d", len(taskRepo.tasks))
	}
}

// TestEnqueueMaintenance_SubCapWeeklyReached verifies that exceeding the weekly
// sub-cap prevents enqueuing, even when the daily cap hasn't been hit yet.
func TestEnqueueMaintenance_SubCapWeeklyReached(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}

	// weekly cap = 3, daily cap = 5 (so daily does not block first)
	mt := config.MaintenanceTaskConfig{
		Name:           "weekly-capped",
		Cron:           "0 3 * * mon",
		Repo:           "owner/repo",
		PromptTemplate: "maintenance/security-scan.tmpl",
		BudgetSubCap:   config.SubCapConfig{Daily: 5, Weekly: 3},
	}

	// Use a budget enforcer that always allows so the sub-cap is the only gate.
	callCount := 0
	budget := &fakeBudgetEnforcer{reason: scheduler.BudgetReasonAllowed}
	uc := makeMaintenanceUC(taskRepo, stateRepo, budget)

	// First 3 calls should succeed (weekly cap = 3).
	// We need to simulate different days to avoid the daily cap of 5.
	// Since we use the same day key in all calls (budget limits use UTC now()),
	// and daily cap is 5, the weekly cap (3) fires after 3 successful calls.
	for i := 0; i < 3; i++ {
		_, err := uc.EnqueueMaintenance(context.Background(), mt)
		if err != nil {
			t.Fatalf("call %d failed unexpectedly: %v", i+1, err)
		}
		callCount++
	}

	// 4th call should fail on weekly sub-cap.
	_, err := uc.EnqueueMaintenance(context.Background(), mt)
	if err == nil {
		t.Fatal("expected weekly sub-cap error on 4th call, got nil")
	}
	if callCount != 3 {
		t.Errorf("expected 3 successful calls, got %d", callCount)
	}
	if len(taskRepo.tasks) != 3 {
		t.Errorf("expected 3 tasks in repo, got %d", len(taskRepo.tasks))
	}
}

// TestEnqueueMaintenance_NoSubCap verifies that when no sub-cap is configured,
// tasks can be enqueued freely.
func TestEnqueueMaintenance_NoSubCap(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}
	budget := &fakeBudgetEnforcer{reason: scheduler.BudgetReasonAllowed}

	mt := config.MaintenanceTaskConfig{
		Name:           "uncapped-task",
		Cron:           "0 4 * * *",
		Repo:           "owner/repo",
		PromptTemplate: "maintenance/dep-update.tmpl",
		BudgetSubCap:   config.SubCapConfig{Daily: 0, Weekly: 0},
	}
	uc := makeMaintenanceUC(taskRepo, stateRepo, budget)

	for i := 0; i < 5; i++ {
		if _, err := uc.EnqueueMaintenance(context.Background(), mt); err != nil {
			t.Fatalf("call %d failed unexpectedly: %v", i+1, err)
		}
	}
	if len(taskRepo.tasks) != 5 {
		t.Errorf("expected 5 tasks in repo, got %d", len(taskRepo.tasks))
	}
}

// TestEnqueueMaintenance_GlobalCountsForGlobalBudget verifies that the global
// budget enforcer is called and counts maintenance tasks.
func TestEnqueueMaintenance_GlobalCountsForGlobalBudget(t *testing.T) {
	taskRepo := &fakeMaintenanceTaskRepo{}
	stateRepo := &fakeMaintenanceAppStateRepo{}

	calls := 0
	budget := &countingBudgetEnforcer{reason: scheduler.BudgetReasonAllowed, counter: &calls}

	mt := config.MaintenanceTaskConfig{
		Name:           "counted-task",
		Cron:           "0 2 * * *",
		Repo:           "owner/repo",
		PromptTemplate: "maintenance/dep-update.tmpl",
	}

	limits := scheduler.BudgetLimits{ResetTZ: time.UTC, WeekStartsOn: time.Monday}
	uc := usecase.NewMaintenanceUseCase(taskRepo, stateRepo, budget, limits)

	if _, err := uc.EnqueueMaintenance(context.Background(), mt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected global budget to be called once, got %d", calls)
	}
}

// countingBudgetEnforcer records how many times CheckAndIncrementReason was called.
type countingBudgetEnforcer struct {
	reason  scheduler.BudgetReason
	counter *int
}

func (c *countingBudgetEnforcer) CheckAndIncrementReason(_ context.Context, _ time.Time) (scheduler.BudgetReason, error) {
	*c.counter++
	return c.reason, nil
}
