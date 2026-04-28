package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
	"github.com/gs97ahn/claude-ops/mocks"
)

// fakeNotifier tracks calls to NotifyCostWarning.
type fakeNotifier struct {
	calls []struct {
		Scope   string
		Percent float64
		Current float64
		Max     float64
	}
}

func (n *fakeNotifier) NotifyCostWarning(_ context.Context, scope string, percent, current, max float64) error {
	n.calls = append(n.calls, struct {
		Scope   string
		Percent float64
		Current float64
		Max     float64
	}{scope, percent, current, max})
	return nil
}

func newBudgetUCWithCost(appState domain.AppStateRepository, usageRepo domain.UsageRepository, notifier usecase.CostWarnNotifier) *usecase.BudgetUseCase {
	uc := usecase.NewBudgetUseCase(appState, scheduler.BudgetLimits{
		WeekStartsOn: time.Monday,
		ResetTZ:      time.UTC,
	})
	uc.WithCostWarn(usageRepo, notifier, 1.0, 5.0)
	return uc
}

func TestEvaluateCostWarn_BelowThreshold_NoNotify(t *testing.T) {
	appState := &fakeAppStateRepo{}
	repo := &mocks.UsageRepository{}
	notifier := &fakeNotifier{}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.70, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(2.0, nil)

	uc := newBudgetUCWithCost(appState, repo, notifier)
	err := uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)
	assert.Len(t, notifier.calls, 0, "should not notify below 80%")
	repo.AssertExpectations(t)
}

func TestEvaluateCostWarn_Daily80Pct_SendsWarning(t *testing.T) {
	appState := &fakeAppStateRepo{}
	repo := &mocks.UsageRepository{}
	notifier := &fakeNotifier{}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.85, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(1.0, nil)

	uc := newBudgetUCWithCost(appState, repo, notifier)
	err := uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)

	dailyNotifications := 0
	for _, c := range notifier.calls {
		if c.Scope == "daily" {
			dailyNotifications++
		}
	}
	assert.Equal(t, 1, dailyNotifications, "should send exactly 1 daily warning")
}

func TestEvaluateCostWarn_Idempotent_SameDay(t *testing.T) {
	appState := &fakeAppStateRepo{}
	repo := &mocks.UsageRepository{}
	notifier := &fakeNotifier{}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	// First call at 85%
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.85, nil).Times(2)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(1.0, nil).Times(2)

	uc := newBudgetUCWithCost(appState, repo, notifier)

	// First evaluation should warn
	err := uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)

	// Second evaluation same day — should NOT warn again
	err = uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)

	dailyNotifications := 0
	for _, c := range notifier.calls {
		if c.Scope == "daily" {
			dailyNotifications++
		}
	}
	assert.Equal(t, 1, dailyNotifications, "idempotent: should not re-send warning on same day")
}

func TestEvaluateCostWarn_Daily100Pct_SendsCapReached(t *testing.T) {
	appState := &fakeAppStateRepo{}
	repo := &mocks.UsageRepository{}
	notifier := &fakeNotifier{}

	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(1.05, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(1.05, nil)

	uc := newBudgetUCWithCost(appState, repo, notifier)
	err := uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)

	// Should fire 100% warnings (both daily and weekly are over)
	// 80% warnings should NOT be sent because 100% takes precedence in the same check
	has100 := false
	for _, c := range notifier.calls {
		if c.Percent >= 100 {
			has100 = true
		}
	}
	assert.True(t, has100, "should have at least one >=100% notification")
}

func TestEvaluateCostWarn_DayRollover_ResetsFlags(t *testing.T) {
	appState := &fakeAppStateRepo{}
	repo := &mocks.UsageRepository{}
	notifier := &fakeNotifier{}

	day1 := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	// Day 1 at 85%
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.85, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(2.0, nil).Times(2)
	// Day 2 at 85% again
	repo.On("SumDailyCost", context.Background(), "2026-04-28").Return(0.85, nil)

	uc := newBudgetUCWithCost(appState, repo, notifier)

	err := uc.EvaluateCostWarn(context.Background(), day1)
	require.NoError(t, err)

	err = uc.EvaluateCostWarn(context.Background(), day2)
	require.NoError(t, err)

	dailyNotifications := 0
	for _, c := range notifier.calls {
		if c.Scope == "daily" {
			dailyNotifications++
		}
	}
	assert.Equal(t, 2, dailyNotifications, "after day rollover flags reset, should warn again")
}

func TestEvaluateCostWarn_MaxZero_NoOp(t *testing.T) {
	appState := &fakeAppStateRepo{}
	notifier := &fakeNotifier{}

	uc := usecase.NewBudgetUseCase(appState, scheduler.BudgetLimits{
		WeekStartsOn: time.Monday,
		ResetTZ:      time.UTC,
	})
	// Do NOT call WithCostWarn — usageRepo and notifier are nil
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	err := uc.EvaluateCostWarn(context.Background(), now)
	require.NoError(t, err)
	assert.Len(t, notifier.calls, 0, "no-op when cost warn not configured")
}
