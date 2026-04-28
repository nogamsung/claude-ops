package usecase

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

const maxRangeDays = 365

// UsageLimitsSnapshot holds cost usage vs configured limits for a point in time.
type UsageLimitsSnapshot struct {
	DailyCountUSD  float64  `json:"count_usd"`
	DailyMaxUSD    float64  `json:"max_usd"`
	DailyPercent   *float64 `json:"percent"`  // nil when max=0
	DailyDate      string   `json:"date"`
	WeeklyCountUSD float64  `json:"weekly_count_usd"`
	WeeklyMaxUSD   float64  `json:"weekly_max_usd"`
	WeeklyPercent  *float64 `json:"weekly_percent"` // nil when max=0
	WeeklyWeek     string   `json:"week"`
}

// UsageAggregateResult holds the full response for GET /usage.
type UsageAggregateResult struct {
	From    string
	To      string
	GroupBy domain.BucketKind
	Buckets []domain.UsageBucketRow
	Totals  domain.UsageBucketRow
}

// UsageUseCase implements aggregation, by-model, and limits queries.
type UsageUseCase struct {
	repo         domain.UsageRepository
	budgetLimits scheduler.BudgetLimits
	dailyMaxCost float64
	weeklyMaxCost float64
}

// NewUsageUseCase creates a new UsageUseCase.
func NewUsageUseCase(repo domain.UsageRepository, budgetLimits scheduler.BudgetLimits, dailyMaxCostUSD, weeklyMaxCostUSD float64) *UsageUseCase {
	return &UsageUseCase{
		repo:          repo,
		budgetLimits:  budgetLimits,
		dailyMaxCost:  dailyMaxCostUSD,
		weeklyMaxCost: weeklyMaxCostUSD,
	}
}

// Aggregate returns time-series usage data for the given range, gap-filled with zero rows.
func (uc *UsageUseCase) Aggregate(ctx context.Context, from, to time.Time, bucket domain.BucketKind) (UsageAggregateResult, error) {
	if err := uc.validateRange(from, to, bucket); err != nil {
		return UsageAggregateResult{}, err
	}

	// The repo uses an exclusive upper bound (SQL < ?); add 1 day so "to" (inclusive) is included.
	toExclusive := to.AddDate(0, 0, 1)
	rows, err := uc.repo.SumByBucket(ctx, from, toExclusive, bucket)
	if err != nil {
		return UsageAggregateResult{}, fmt.Errorf("aggregate usage: %w", err)
	}

	filled := uc.gapFill(from, to, bucket, rows)
	totals := computeTotals(filled)

	return UsageAggregateResult{
		From:    from.Format("2006-01-02"),
		To:      to.Format("2006-01-02"),
		GroupBy: bucket,
		Buckets: filled,
		Totals:  totals,
	}, nil
}

// ByModel returns per-model aggregated usage sorted by cost descending.
func (uc *UsageUseCase) ByModel(ctx context.Context, from, to time.Time) ([]domain.UsageModelRow, error) {
	if err := validateDateRange(from, to); err != nil { // MODIFIED: added ErrRangeTooLarge check
		return nil, err
	}
	// The repo uses an exclusive upper bound; add 1 day so "to" (inclusive) is included.
	toExclusive := to.AddDate(0, 0, 1)
	rows, err := uc.repo.SumByModel(ctx, from, toExclusive)
	if err != nil {
		return nil, fmt.Errorf("by model usage: %w", err)
	}
	return rows, nil
}

// Limits returns the current cost usage vs configured cost limits.
func (uc *UsageUseCase) Limits(ctx context.Context, now time.Time) (UsageLimitsSnapshot, error) {
	tz := uc.budgetLimits.ResetTZ
	weekStart := uc.budgetLimits.WeekStartsOn

	dayKey := scheduler.DateKey(now, tz)
	weekKey := scheduler.WeekKey(now, tz, weekStart)

	dailyCost, err := uc.repo.SumDailyCost(ctx, dayKey)
	if err != nil {
		return UsageLimitsSnapshot{}, fmt.Errorf("sum daily cost: %w", err)
	}
	weeklyCost, err := uc.repo.SumWeeklyCost(ctx, weekKey)
	if err != nil {
		return UsageLimitsSnapshot{}, fmt.Errorf("sum weekly cost: %w", err)
	}

	snap := UsageLimitsSnapshot{
		DailyCountUSD:  dailyCost,
		DailyMaxUSD:    uc.dailyMaxCost,
		DailyDate:      dayKey,
		WeeklyCountUSD: weeklyCost,
		WeeklyMaxUSD:   uc.weeklyMaxCost,
		WeeklyWeek:     weekKey,
	}

	if uc.dailyMaxCost > 0 {
		pct := math.Round(dailyCost/uc.dailyMaxCost*100*10) / 10
		if pct > 100 {
			pct = 100
		}
		snap.DailyPercent = &pct
	}
	if uc.weeklyMaxCost > 0 {
		pct := math.Round(weeklyCost/uc.weeklyMaxCost*100*10) / 10
		if pct > 100 {
			pct = 100
		}
		snap.WeeklyPercent = &pct
	}

	return snap, nil
}

// validateDateRange checks that from <= to and the span does not exceed maxRangeDays. // ADDED
func validateDateRange(from, to time.Time) error { // ADDED
	if from.After(to) { // ADDED
		return domain.ErrInvalidRange // ADDED
	} // ADDED
	if to.Sub(from) > maxRangeDays*24*time.Hour { // ADDED
		return domain.ErrRangeTooLarge // ADDED
	} // ADDED
	return nil // ADDED
} // ADDED

func (uc *UsageUseCase) validateRange(from, to time.Time, bucket domain.BucketKind) error {
	if err := validateDateRange(from, to); err != nil { // MODIFIED: delegate to validateDateRange
		return err
	}
	switch bucket {
	case domain.BucketDay, domain.BucketWeek, domain.BucketMonth:
	default:
		return domain.ErrInvalidBucket
	}
	return nil
}

// gapFill ensures every bucket in [from, to] is present in the result, zero-filling missing ones.
func (uc *UsageUseCase) gapFill(from, to time.Time, bucket domain.BucketKind, rows []domain.UsageBucketRow) []domain.UsageBucketRow {
	// Build a lookup map by bucket key.
	byKey := make(map[string]domain.UsageBucketRow, len(rows))
	for _, r := range rows {
		byKey[r.Bucket] = r
	}

	var result []domain.UsageBucketRow
	switch bucket {
	case domain.BucketDay:
		for cur := from; !cur.After(to); cur = cur.AddDate(0, 0, 1) {
			key := cur.Format("2006-01-02")
			if row, ok := byKey[key]; ok {
				result = append(result, row)
			} else {
				result = append(result, domain.UsageBucketRow{Bucket: key})
			}
		}
	case domain.BucketWeek:
		tz := uc.budgetLimits.ResetTZ
		ws := uc.budgetLimits.WeekStartsOn
		cur := from
		seen := make(map[string]bool)
		for !cur.After(to) {
			key := scheduler.WeekKey(cur, tz, ws)
			if !seen[key] {
				seen[key] = true
				if row, ok := byKey[key]; ok {
					result = append(result, row)
				} else {
					result = append(result, domain.UsageBucketRow{Bucket: key})
				}
			}
			cur = cur.AddDate(0, 0, 1)
		}
	case domain.BucketMonth:
		cur := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
		end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, to.Location())
		for !cur.After(end) {
			key := cur.Format("2006-01")
			if row, ok := byKey[key]; ok {
				result = append(result, row)
			} else {
				result = append(result, domain.UsageBucketRow{Bucket: key})
			}
			cur = cur.AddDate(0, 1, 0)
		}
	}

	return result
}

func computeTotals(rows []domain.UsageBucketRow) domain.UsageBucketRow {
	var t domain.UsageBucketRow
	for _, r := range rows {
		t.TaskCount += r.TaskCount
		t.CostUSD += r.CostUSD
		t.InputTokens += r.InputTokens
		t.OutputTokens += r.OutputTokens
		t.CacheReadTokens += r.CacheReadTokens
		t.CacheCreationTokens += r.CacheCreationTokens
		t.FailedCostUSD += r.FailedCostUSD
	}
	return t
}
