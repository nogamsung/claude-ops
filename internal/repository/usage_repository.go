package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sqlcdb "github.com/gs97ahn/claude-ops/db/sqlc"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// modelUsageEntry mirrors stream.ModelUsage for JSON unmarshalling.
type modelUsageEntry struct {
	InputTokens              int64   `json:"inputTokens"`
	OutputTokens             int64   `json:"outputTokens"`
	CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`
	CostUSD                  float64 `json:"costUSD"`
}

// SQLiteUsageRepository implements domain.UsageRepository using sqlc.
// Name kept for backward compatibility; renamed to GormUsageRepository in a follow-up commit.
type SQLiteUsageRepository struct {
	queries *sqlcdb.Queries
}

// NewSQLiteUsageRepository creates a new SQLiteUsageRepository.
func NewSQLiteUsageRepository(queries *sqlcdb.Queries) *SQLiteUsageRepository {
	return &SQLiteUsageRepository{queries: queries}
}

// SumByBucket aggregates usage data grouped by the specified bucket granularity.
func (r *SQLiteUsageRepository) SumByBucket(ctx context.Context, from, to time.Time, bucket domain.BucketKind) ([]domain.UsageBucketRow, error) {
	switch bucket {
	case domain.BucketDay:
		return r.sumByDay(ctx, from, to)
	case domain.BucketWeek:
		return r.sumByWeek(ctx, from, to)
	case domain.BucketMonth:
		return r.sumByMonth(ctx, from, to)
	default:
		return nil, domain.ErrInvalidBucket
	}
}

func (r *SQLiteUsageRepository) sumByDay(ctx context.Context, from, to time.Time) ([]domain.UsageBucketRow, error) {
	rows, err := r.queries.SumUsageByDay(ctx, sqlcdb.SumUsageByDayParams{
		FromTs: nullTime(from),
		ToTs:   nullTime(to),
	})
	if err != nil {
		return nil, fmt.Errorf("sum usage by day: %w", err)
	}
	out := make([]domain.UsageBucketRow, len(rows))
	for i, r := range rows {
		out[i] = domain.UsageBucketRow{
			Bucket:              r.Bucket,
			TaskCount:           r.TaskCount,
			CostUSD:             r.CostUsd,
			InputTokens:         r.InputTokens,
			OutputTokens:        r.OutputTokens,
			CacheReadTokens:     r.CacheReadTokens,
			CacheCreationTokens: r.CacheCreationTokens,
			FailedCostUSD:       r.FailedCostUsd,
		}
	}
	return out, nil
}

func (r *SQLiteUsageRepository) sumByWeek(ctx context.Context, from, to time.Time) ([]domain.UsageBucketRow, error) {
	rows, err := r.queries.SumUsageByWeek(ctx, sqlcdb.SumUsageByWeekParams{
		FromTs: nullTime(from),
		ToTs:   nullTime(to),
	})
	if err != nil {
		return nil, fmt.Errorf("sum usage by week: %w", err)
	}
	out := make([]domain.UsageBucketRow, len(rows))
	for i, r := range rows {
		out[i] = domain.UsageBucketRow{
			Bucket:              r.Bucket,
			TaskCount:           r.TaskCount,
			CostUSD:             r.CostUsd,
			InputTokens:         r.InputTokens,
			OutputTokens:        r.OutputTokens,
			CacheReadTokens:     r.CacheReadTokens,
			CacheCreationTokens: r.CacheCreationTokens,
			FailedCostUSD:       r.FailedCostUsd,
		}
	}
	return out, nil
}

func (r *SQLiteUsageRepository) sumByMonth(ctx context.Context, from, to time.Time) ([]domain.UsageBucketRow, error) {
	rows, err := r.queries.SumUsageByMonth(ctx, sqlcdb.SumUsageByMonthParams{
		FromTs: nullTime(from),
		ToTs:   nullTime(to),
	})
	if err != nil {
		return nil, fmt.Errorf("sum usage by month: %w", err)
	}
	out := make([]domain.UsageBucketRow, len(rows))
	for i, r := range rows {
		out[i] = domain.UsageBucketRow{
			Bucket:              r.Bucket,
			TaskCount:           r.TaskCount,
			CostUSD:             r.CostUsd,
			InputTokens:         r.InputTokens,
			OutputTokens:        r.OutputTokens,
			CacheReadTokens:     r.CacheReadTokens,
			CacheCreationTokens: r.CacheCreationTokens,
			FailedCostUSD:       r.FailedCostUsd,
		}
	}
	return out, nil
}

// SumByModel aggregates per-model usage for done tasks via application-side JSON expansion.
// PRD §10 R2 — JSON storage + application-side scan is OK for v1 (≤10k tasks).
func (r *SQLiteUsageRepository) SumByModel(ctx context.Context, from, to time.Time) ([]domain.UsageModelRow, error) {
	rows, err := r.queries.SumUsageByModel(ctx, sqlcdb.SumUsageByModelParams{
		FromTs: nullTime(from),
		ToTs:   nullTime(to),
	})
	if err != nil {
		return nil, fmt.Errorf("sum usage by model: %w", err)
	}

	type acc struct {
		TaskCount           int64
		CostUSD             float64
		InputTokens         int64
		OutputTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
	}
	aggregated := make(map[string]*acc)

	for _, row := range rows {
		var modelMap map[string]modelUsageEntry
		if err := json.Unmarshal(row.ModelUsageJson, &modelMap); err != nil || len(modelMap) == 0 {
			key := "unknown"
			if _, ok := aggregated[key]; !ok {
				aggregated[key] = &acc{}
			}
			a := aggregated[key]
			a.TaskCount++
			a.CostUSD += row.CostUsd
			a.InputTokens += row.TotalInputTokens
			a.OutputTokens += row.TotalOutputTokens
			a.CacheReadTokens += row.CacheReadInputTokens
			a.CacheCreationTokens += row.CacheCreationInputTokens
			continue
		}

		for modelID, usage := range modelMap {
			key := modelID
			if key == "" {
				key = "unknown"
			}
			if _, ok := aggregated[key]; !ok {
				aggregated[key] = &acc{}
			}
			a := aggregated[key]
			a.TaskCount++
			a.CostUSD += usage.CostUSD
			a.InputTokens += usage.InputTokens
			a.OutputTokens += usage.OutputTokens
			a.CacheReadTokens += usage.CacheReadInputTokens
			a.CacheCreationTokens += usage.CacheCreationInputTokens
		}
	}

	result := make([]domain.UsageModelRow, 0, len(aggregated))
	for modelID, a := range aggregated {
		result = append(result, domain.UsageModelRow{
			ModelID:             modelID,
			TaskCount:           a.TaskCount,
			CostUSD:             a.CostUSD,
			InputTokens:         a.InputTokens,
			OutputTokens:        a.OutputTokens,
			CacheReadTokens:     a.CacheReadTokens,
			CacheCreationTokens: a.CacheCreationTokens,
		})
	}

	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].CostUSD > result[i].CostUSD {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// SumDailyCost returns the total cost_usd for done tasks on the given day key (YYYY-MM-DD).
// The day boundary is resolved in UTC; callers that need a different tz should
// adjust the key generator accordingly.
func (r *SQLiteUsageRepository) SumDailyCost(ctx context.Context, dayKey string) (float64, error) {
	start, end, err := dayKeyToRange(dayKey)
	if err != nil {
		return 0, fmt.Errorf("sum daily cost parse key %q: %w", dayKey, err)
	}
	total, err := r.queries.SumDailyCost(ctx, sqlcdb.SumDailyCostParams{
		DayStart: nullTime(start),
		DayEnd:   nullTime(end),
	})
	if err != nil {
		return 0, fmt.Errorf("sum daily cost: %w", err)
	}
	return total, nil
}

// SumWeeklyCost returns the total cost_usd for done tasks in the given ISO 8601 week key (YYYY-Www).
func (r *SQLiteUsageRepository) SumWeeklyCost(ctx context.Context, weekKey string) (float64, error) {
	start, end, err := weekKeyToRange(weekKey)
	if err != nil {
		return 0, fmt.Errorf("sum weekly cost parse key %q: %w", weekKey, err)
	}
	total, err := r.queries.SumWeeklyCost(ctx, sqlcdb.SumWeeklyCostParams{
		WeekStart: nullTime(start),
		WeekEnd:   nullTime(end),
	})
	if err != nil {
		return 0, fmt.Errorf("sum weekly cost: %w", err)
	}
	return total, nil
}

// nullTime wraps a time.Time as sql.NullTime{Valid: true}. Zero times stay invalid.
func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// dayKeyToRange converts a "YYYY-MM-DD" key to a [start, end) UTC day range.
func dayKeyToRange(dayKey string) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", dayKey, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse day key: %w", err)
	}
	return t, t.AddDate(0, 0, 1), nil
}

// weekKeyToRange converts a "YYYY-Www" key (ISO 8601) to a [start, end) UTC week range
// anchored on Monday.
func weekKeyToRange(weekKey string) (time.Time, time.Time, error) {
	var year, week int
	if _, err := fmt.Sscanf(weekKey, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse week key: %w", err)
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	_, jan4Week := jan4.ISOWeek()
	jan4Mon := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday+7)%7)
	weekStart := jan4Mon.AddDate(0, 0, (week-jan4Week)*7)
	weekEnd := weekStart.AddDate(0, 0, 7)
	return weekStart, weekEnd, nil
}
