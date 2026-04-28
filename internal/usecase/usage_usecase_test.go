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

func newUseCase(repo domain.UsageRepository) *usecase.UsageUseCase {
	limits := scheduler.BudgetLimits{
		WeekStartsOn: time.Monday,
		ResetTZ:      time.UTC,
	}
	return usecase.NewUsageUseCase(repo, limits, 1.0, 5.0)
}

func TestAggregate_HappyPath_Day(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	rows := []domain.UsageBucketRow{
		{Bucket: "2026-04-26", TaskCount: 2, CostUSD: 0.50, InputTokens: 1000, OutputTokens: 500},
		{Bucket: "2026-04-27", TaskCount: 1, CostUSD: 0.25, InputTokens: 500, OutputTokens: 250},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketDay).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketDay)
	require.NoError(t, err)
	assert.Equal(t, "2026-04-26", result.From)
	assert.Equal(t, "2026-04-27", result.To)
	assert.Equal(t, domain.BucketDay, result.GroupBy)
	assert.Len(t, result.Buckets, 2)
	assert.InDelta(t, 0.75, result.Totals.CostUSD, 0.001)
	assert.Equal(t, int64(3), result.Totals.TaskCount)
	repo.AssertExpectations(t)
}

func TestAggregate_GapFill_Day(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	// Only return 2 rows — the middle day (2026-04-26) is missing
	rows := []domain.UsageBucketRow{
		{Bucket: "2026-04-25", TaskCount: 1, CostUSD: 0.10},
		{Bucket: "2026-04-27", TaskCount: 1, CostUSD: 0.20},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketDay).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketDay)
	require.NoError(t, err)
	assert.Len(t, result.Buckets, 3, "gap-fill should produce 3 buckets")
	assert.Equal(t, "2026-04-25", result.Buckets[0].Bucket)
	assert.Equal(t, "2026-04-26", result.Buckets[1].Bucket)
	assert.Equal(t, float64(0), result.Buckets[1].CostUSD, "gap bucket should be zero")
	assert.Equal(t, "2026-04-27", result.Buckets[2].Bucket)
	repo.AssertExpectations(t)
}

func TestAggregate_ErrInvalidRange(t *testing.T) {
	repo := &mocks.UsageRepository{}
	uc := newUseCase(repo)
	from := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := uc.Aggregate(context.Background(), from, to, domain.BucketDay)
	assert.ErrorIs(t, err, domain.ErrInvalidRange)
	repo.AssertNotCalled(t, "SumByBucket")
}

func TestAggregate_ErrRangeTooLarge(t *testing.T) {
	repo := &mocks.UsageRepository{}
	uc := newUseCase(repo)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC) // > 365 days

	_, err := uc.Aggregate(context.Background(), from, to, domain.BucketDay)
	assert.ErrorIs(t, err, domain.ErrRangeTooLarge)
}

func TestAggregate_ErrInvalidBucket(t *testing.T) {
	repo := &mocks.UsageRepository{}
	uc := newUseCase(repo)
	from := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)

	_, err := uc.Aggregate(context.Background(), from, to, domain.BucketKind("year"))
	assert.ErrorIs(t, err, domain.ErrInvalidBucket)
}

func TestAggregate_HappyPath_Week(t *testing.T) {
	repo := &mocks.UsageRepository{}
	// 2026-04-20 is in week 2026-W17, 2026-04-27 is in week 2026-W18
	from := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	rows := []domain.UsageBucketRow{
		{Bucket: "2026-W17", TaskCount: 5, CostUSD: 1.00},
		{Bucket: "2026-W18", TaskCount: 3, CostUSD: 0.60},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketWeek).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketWeek)
	require.NoError(t, err)
	assert.InDelta(t, 1.60, result.Totals.CostUSD, 0.001)
	assert.Equal(t, int64(8), result.Totals.TaskCount)
	repo.AssertExpectations(t)
}

func TestByModel_PassThrough(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	// Repo is responsible for sorting; usecase passes through the result.
	rows := []domain.UsageModelRow{
		{ModelID: "claude-opus-4-5", CostUSD: 10.00},
		{ModelID: "claude-sonnet-4-5", CostUSD: 2.50},
		{ModelID: "unknown", CostUSD: 0.50},
	}
	repo.On("SumByModel", context.Background(), from, toExclusive).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.ByModel(context.Background(), from, to)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, "claude-opus-4-5", result[0].ModelID, "repo-sorted result passed through")
	repo.AssertExpectations(t)
}

func TestLimits_WithMaxSet(t *testing.T) {
	repo := &mocks.UsageRepository{}
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.85, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(4.20, nil)

	uc := newUseCase(repo)
	snap, err := uc.Limits(context.Background(), now)
	require.NoError(t, err)
	assert.InDelta(t, 0.85, snap.DailyCountUSD, 0.001)
	assert.Equal(t, 1.0, snap.DailyMaxUSD)
	require.NotNil(t, snap.DailyPercent)
	assert.InDelta(t, 85.0, *snap.DailyPercent, 0.5)
	assert.Equal(t, "2026-04-27", snap.DailyDate)
	assert.InDelta(t, 4.20, snap.WeeklyCountUSD, 0.001)
	require.NotNil(t, snap.WeeklyPercent)
	assert.InDelta(t, 84.0, *snap.WeeklyPercent, 0.5)
	repo.AssertExpectations(t)
}

func TestLimits_MaxZeroReturnsNilPercent(t *testing.T) {
	repo := &mocks.UsageRepository{}
	limits := scheduler.BudgetLimits{
		WeekStartsOn: time.Monday,
		ResetTZ:      time.UTC,
	}
	uc := usecase.NewUsageUseCase(repo, limits, 0, 0) // no cost limits
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(float64(0), nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(float64(0), nil)

	snap, err := uc.Limits(context.Background(), now)
	require.NoError(t, err)
	assert.Nil(t, snap.DailyPercent, "percent should be nil when max=0")
	assert.Nil(t, snap.WeeklyPercent, "percent should be nil when max=0")
	repo.AssertExpectations(t)
}

func TestAggregate_HappyPath_Month(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	rows := []domain.UsageBucketRow{
		{Bucket: "2026-03", TaskCount: 10, CostUSD: 2.00},
		{Bucket: "2026-04", TaskCount: 5, CostUSD: 1.00},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketMonth).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketMonth)
	require.NoError(t, err)
	assert.Len(t, result.Buckets, 2)
	assert.InDelta(t, 3.00, result.Totals.CostUSD, 0.001)
	assert.Equal(t, int64(15), result.Totals.TaskCount)
	repo.AssertExpectations(t)
}

func TestAggregate_GapFill_Month(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	// Only return Jan and Mar — Feb is missing
	rows := []domain.UsageBucketRow{
		{Bucket: "2026-01", TaskCount: 3, CostUSD: 0.30},
		{Bucket: "2026-03", TaskCount: 2, CostUSD: 0.20},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketMonth).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketMonth)
	require.NoError(t, err)
	assert.Len(t, result.Buckets, 3, "gap-fill should produce Jan, Feb, Mar")
	assert.Equal(t, "2026-01", result.Buckets[0].Bucket)
	assert.Equal(t, "2026-02", result.Buckets[1].Bucket)
	assert.Equal(t, float64(0), result.Buckets[1].CostUSD, "gap bucket should be zero")
	assert.Equal(t, "2026-03", result.Buckets[2].Bucket)
	repo.AssertExpectations(t)
}

func TestAggregate_GapFill_Week(t *testing.T) {
	repo := &mocks.UsageRepository{}
	// 2026-04-13 (Mon W16) to 2026-04-27 (Mon W18) — W17 is missing
	from := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	rows := []domain.UsageBucketRow{
		{Bucket: "2026-W16", TaskCount: 4, CostUSD: 0.80},
		{Bucket: "2026-W18", TaskCount: 2, CostUSD: 0.40},
	}
	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketWeek).Return(rows, nil)

	uc := newUseCase(repo)
	result, err := uc.Aggregate(context.Background(), from, to, domain.BucketWeek)
	require.NoError(t, err)
	assert.Len(t, result.Buckets, 3, "gap-fill should produce W16, W17, W18")
	assert.Equal(t, "2026-W16", result.Buckets[0].Bucket)
	assert.Equal(t, "2026-W17", result.Buckets[1].Bucket)
	assert.Equal(t, float64(0), result.Buckets[1].CostUSD, "gap bucket should be zero")
	assert.Equal(t, "2026-W18", result.Buckets[2].Bucket)
	repo.AssertExpectations(t)
}

func TestAggregate_RepoError(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	repo.On("SumByBucket", context.Background(), from, toExclusive, domain.BucketDay).
		Return([]domain.UsageBucketRow(nil), assert.AnError)

	uc := newUseCase(repo)
	_, err := uc.Aggregate(context.Background(), from, to, domain.BucketDay)
	assert.Error(t, err)
	repo.AssertExpectations(t)
}

func TestByModel_InvalidRange(t *testing.T) {
	repo := &mocks.UsageRepository{}
	uc := newUseCase(repo)
	from := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := uc.ByModel(context.Background(), from, to)
	assert.ErrorIs(t, err, domain.ErrInvalidRange)
	repo.AssertNotCalled(t, "SumByModel")
}

func TestByModel_RangeTooLarge(t *testing.T) { // ADDED
	repo := &mocks.UsageRepository{}
	uc := newUseCase(repo)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC) // > 365 days

	_, err := uc.ByModel(context.Background(), from, to)
	assert.ErrorIs(t, err, domain.ErrRangeTooLarge)
	repo.AssertNotCalled(t, "SumByModel")
}

func TestByModel_RepoError(t *testing.T) {
	repo := &mocks.UsageRepository{}
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	toExclusive := to.AddDate(0, 0, 1)

	repo.On("SumByModel", context.Background(), from, toExclusive).
		Return([]domain.UsageModelRow(nil), assert.AnError)

	uc := newUseCase(repo)
	_, err := uc.ByModel(context.Background(), from, to)
	assert.Error(t, err)
	repo.AssertExpectations(t)
}

func TestLimits_PercentCappedAt100(t *testing.T) {
	repo := &mocks.UsageRepository{}
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	// Cost exceeds max — percent should be capped at 100
	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(2.0, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(10.0, nil)

	uc := newUseCase(repo) // dailyMax=1.0, weeklyMax=5.0
	snap, err := uc.Limits(context.Background(), now)
	require.NoError(t, err)
	require.NotNil(t, snap.DailyPercent)
	assert.Equal(t, 100.0, *snap.DailyPercent, "percent capped at 100 when over budget")
	require.NotNil(t, snap.WeeklyPercent)
	assert.Equal(t, 100.0, *snap.WeeklyPercent, "percent capped at 100 when over budget")
	repo.AssertExpectations(t)
}

func TestLimits_DailyRepoError(t *testing.T) {
	repo := &mocks.UsageRepository{}
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(float64(0), assert.AnError)

	uc := newUseCase(repo)
	_, err := uc.Limits(context.Background(), now)
	assert.Error(t, err)
	repo.AssertExpectations(t)
}

func TestLimits_WeeklyRepoError(t *testing.T) {
	repo := &mocks.UsageRepository{}
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	repo.On("SumDailyCost", context.Background(), "2026-04-27").Return(0.5, nil)
	repo.On("SumWeeklyCost", context.Background(), "2026-W18").Return(float64(0), assert.AnError)

	uc := newUseCase(repo)
	_, err := uc.Limits(context.Background(), now)
	assert.Error(t, err)
	repo.AssertExpectations(t)
}
