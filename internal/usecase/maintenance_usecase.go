package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gs97ahn/claude-ops/internal/config"
	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

// maintenanceSubCapJSON is the persisted shape of per-task-name sub-cap counters.
type maintenanceSubCapJSON struct {
	DailyCount  int    `json:"daily_count"`
	DailyKey    string `json:"daily_key"`
	WeeklyCount int    `json:"weekly_count"`
	WeeklyKey   string `json:"weekly_key"`
}

// appStateKeyMaintenanceCounters returns the AppState key for a named task's sub-cap counters.
func appStateKeyMaintenanceCounters(name string) string {
	return "maintenance_counters:" + name
}

// MaintenanceBudgetEnforcer is the subset of BudgetUseCase needed by maintenance.
type MaintenanceBudgetEnforcer interface {
	CheckAndIncrementReason(ctx context.Context, now time.Time) (scheduler.BudgetReason, error)
}

// MaintenanceUseCase manages cron-triggered maintenance task enqueuing.
//
// It enforces:
//  1. The global budget gate (delegates to BudgetUseCase).
//  2. Per-task-name sub-cap counters stored in AppState.
type MaintenanceUseCase struct {
	taskRepo     domain.TaskRepository
	appStateRepo domain.AppStateRepository
	budget       MaintenanceBudgetEnforcer
	limits       scheduler.BudgetLimits
	mu           sync.Mutex
}

// NewMaintenanceUseCase creates a MaintenanceUseCase.
func NewMaintenanceUseCase(
	taskRepo domain.TaskRepository,
	appStateRepo domain.AppStateRepository,
	budget MaintenanceBudgetEnforcer,
	limits scheduler.BudgetLimits,
) *MaintenanceUseCase {
	return &MaintenanceUseCase{
		taskRepo:     taskRepo,
		appStateRepo: appStateRepo,
		budget:       budget,
		limits:       limits,
	}
}

// ErrSubCapReached is returned when a maintenance task's own sub-cap is exhausted.
var ErrSubCapReached = errors.New("maintenance sub-cap reached")

// EnqueueMaintenance enqueues a maintenance task if all budget gates pass.
// Returns ErrSubCapReached when the task's own daily/weekly sub-cap is exhausted.
// Returns a non-nil BudgetReason string wrapped as an error when the global budget blocks.
func (uc *MaintenanceUseCase) EnqueueMaintenance(ctx context.Context, mt config.MaintenanceTaskConfig) (*domain.Task, error) {
	now := time.Now()

	// 1. Global budget gate — maintenance counts toward the global cap.
	reason, err := uc.budget.CheckAndIncrementReason(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("check global budget: %w", err)
	}
	if reason != scheduler.BudgetReasonAllowed {
		return nil, fmt.Errorf("global budget blocked: %s", string(reason))
	}

	// 2. Per-task sub-cap enforcement (serialised with mutex).
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if err := uc.checkAndIncrementSubCap(ctx, mt, now); err != nil {
		return nil, err
	}

	// 3. Create the task.
	task := &domain.Task{
		ID:              uuid.New().String(),
		RepoFullName:    mt.Repo,
		IssueNumber:     0,
		IssueTitle:      fmt.Sprintf("Maintenance: %s", mt.Name),
		TaskType:        domain.TaskTypeFeature,
		Status:          domain.TaskStatusQueued,
		Source:          domain.TaskSourceMaintenance,
		MaintenanceName: mt.Name,
		PromptTemplate:  mt.PromptTemplate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := uc.taskRepo.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("create maintenance task: %w", err)
	}
	return task, nil
}

// checkAndIncrementSubCap loads, checks, and increments the per-name sub-cap counter.
// Must be called with uc.mu held.
func (uc *MaintenanceUseCase) checkAndIncrementSubCap(ctx context.Context, mt config.MaintenanceTaskConfig, now time.Time) error {
	if mt.BudgetSubCap.Daily == 0 && mt.BudgetSubCap.Weekly == 0 {
		// No sub-cap configured — skip.
		return nil
	}

	key := appStateKeyMaintenanceCounters(mt.Name)
	counters, err := uc.loadSubCap(ctx, key)
	if err != nil {
		return fmt.Errorf("load sub-cap for %q: %w", mt.Name, err)
	}

	// Rollover stale buckets.
	dailyKey := scheduler.DateKey(now, uc.limits.ResetTZ)
	weeklyKey := scheduler.WeekKey(now, uc.limits.ResetTZ, uc.limits.WeekStartsOn)
	if counters.DailyKey != dailyKey {
		counters.DailyKey = dailyKey
		counters.DailyCount = 0
	}
	if counters.WeeklyKey != weeklyKey {
		counters.WeeklyKey = weeklyKey
		counters.WeeklyCount = 0
	}

	// Enforce caps.
	if mt.BudgetSubCap.Daily > 0 && counters.DailyCount >= mt.BudgetSubCap.Daily {
		return fmt.Errorf("%w: %q daily limit %d reached", ErrSubCapReached, mt.Name, mt.BudgetSubCap.Daily)
	}
	if mt.BudgetSubCap.Weekly > 0 && counters.WeeklyCount >= mt.BudgetSubCap.Weekly {
		return fmt.Errorf("%w: %q weekly limit %d reached", ErrSubCapReached, mt.Name, mt.BudgetSubCap.Weekly)
	}

	// Increment and persist.
	counters.DailyCount++
	counters.WeeklyCount++
	return uc.persistSubCap(ctx, key, counters, now)
}

func (uc *MaintenanceUseCase) loadSubCap(ctx context.Context, key string) (maintenanceSubCapJSON, error) {
	state, err := uc.appStateRepo.Get(ctx, key)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return maintenanceSubCapJSON{}, nil
		}
		return maintenanceSubCapJSON{}, err
	}
	var c maintenanceSubCapJSON
	if jsonErr := json.Unmarshal([]byte(state.ValueJSON), &c); jsonErr != nil {
		// Corrupted state — start fresh.
		return maintenanceSubCapJSON{}, nil
	}
	return c, nil
}

func (uc *MaintenanceUseCase) persistSubCap(ctx context.Context, key string, c maintenanceSubCapJSON, now time.Time) error {
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal sub-cap: %w", err)
	}
	return uc.appStateRepo.Set(ctx, &domain.AppState{
		Key:       key,
		ValueJSON: string(b),
		UpdatedAt: now,
	})
}
