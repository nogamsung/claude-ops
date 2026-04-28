package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gs97ahn/claude-ops/db/sqlc"
	"github.com/gs97ahn/claude-ops/internal/domain"
)

// modelUsageEntry mirrors stream.ModelUsage for JSON unmarshalling.
type modelUsageEntry struct {
	InputTokens              int64   `json:"inputTokens"`               // MODIFIED: int → int64
	OutputTokens             int64   `json:"outputTokens"`              // MODIFIED: int → int64
	CacheReadInputTokens     int64   `json:"cacheReadInputTokens"`      // MODIFIED: int → int64
	CacheCreationInputTokens int64   `json:"cacheCreationInputTokens"`  // MODIFIED: int → int64
	CostUSD                  float64 `json:"costUSD"`
}

// SQLiteUsageRepository implements domain.UsageRepository using sqlc.
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
	rows, err := r.queries.SumUsageByDay(ctx, sqlcdb.SumUsageByDayParams{From: from, To: to})
	if err != nil {
		return nil, fmt.Errorf("sum usage by day: %w", err)
	}
	return mapBucketRows(rows), nil
}

func (r *SQLiteUsageRepository) sumByWeek(ctx context.Context, from, to time.Time) ([]domain.UsageBucketRow, error) {
	rows, err := r.queries.SumUsageByWeek(ctx, sqlcdb.SumUsageByWeekParams{From: from, To: to})
	if err != nil {
		return nil, fmt.Errorf("sum usage by week: %w", err)
	}
	return mapBucketRows(rows), nil
}

func (r *SQLiteUsageRepository) sumByMonth(ctx context.Context, from, to time.Time) ([]domain.UsageBucketRow, error) {
	rows, err := r.queries.SumUsageByMonth(ctx, sqlcdb.SumUsageByMonthParams{From: from, To: to})
	if err != nil {
		return nil, fmt.Errorf("sum usage by month: %w", err)
	}
	return mapBucketRows(rows), nil
}

func mapBucketRows(rows []sqlcdb.SumUsageByDayRow) []domain.UsageBucketRow {
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
	return out
}

// SumByModel aggregates per-model usage for done tasks via application-side JSON expansion.
// PRD §10 R2 — TEXT storage + application-side scan is OK for v1 (≤10k tasks).
func (r *SQLiteUsageRepository) SumByModel(ctx context.Context, from, to time.Time) ([]domain.UsageModelRow, error) {
	rows, err := r.queries.SumUsageByModel(ctx, sqlcdb.SumUsageByModelParams{From: from, To: to})
	if err != nil {
		return nil, fmt.Errorf("sum usage by model: %w", err)
	}

	// Application-side aggregation keyed by model_id.
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
		if err := json.Unmarshal([]byte(row.ModelUsageJson), &modelMap); err != nil || len(modelMap) == 0 {
			// No model breakdown — attribute to "unknown" using aggregate columns.
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

		// Expand model breakdown from JSON.
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
			a.InputTokens += usage.InputTokens               // MODIFIED: removed int64() cast
			a.OutputTokens += usage.OutputTokens             // MODIFIED: removed int64() cast
			a.CacheReadTokens += usage.CacheReadInputTokens  // MODIFIED: removed int64() cast
			a.CacheCreationTokens += usage.CacheCreationInputTokens // MODIFIED: removed int64() cast
		}
	}

	// Convert map to slice and sort by cost_usd descending.
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

	// Sort by cost_usd descending (bubble-free using sort).
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
func (r *SQLiteUsageRepository) SumDailyCost(ctx context.Context, dayKey string) (float64, error) {
	total, err := r.queries.SumDailyCost(ctx, dayKey)
	if err != nil {
		return 0, fmt.Errorf("sum daily cost: %w", err)
	}
	return total, nil
}

// SumWeeklyCost returns the total cost_usd for done tasks in the given week key (YYYY-Www).
// The weekKey is expected in the same format as scheduler.WeekKey output.
// We convert the key to a 7-day date range to avoid SQLite strftime week-numbering differences.
func (r *SQLiteUsageRepository) SumWeeklyCost(ctx context.Context, weekKey string) (float64, error) {
	from, to, err := weekKeyToRange(weekKey)
	if err != nil {
		return 0, fmt.Errorf("sum weekly cost parse key %q: %w", weekKey, err)
	}
	rows, err := r.queries.SumUsageByDay(ctx, sqlcdb.SumUsageByDayParams{From: from, To: to})
	if err != nil {
		return 0, fmt.Errorf("sum weekly cost: %w", err)
	}
	var total float64
	for _, row := range rows {
		total += row.CostUsd
	}
	return total, nil
}

// weekKeyToRange converts a "YYYY-Www" key (as produced by scheduler.WeekKey) to a
// [from, to) date range covering the 7 days of that week.
// The week is identified by parsing the year and week number, then anchoring on Monday.
func weekKeyToRange(weekKey string) (time.Time, time.Time, error) {
	var year, week int
	if _, err := fmt.Sscanf(weekKey, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse week key: %w", err)
	}
	// Find the Monday of ISO week `week` in `year`.
	// January 4 is always in ISO week 1.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	_, jan4Week := jan4.ISOWeek()
	// Snap jan4 to its Monday.
	jan4Mon := jan4.AddDate(0, 0, -int(jan4.Weekday()-time.Monday+7)%7)
	weekStart := jan4Mon.AddDate(0, 0, (week-jan4Week)*7)
	weekEnd := weekStart.AddDate(0, 0, 7)
	return weekStart, weekEnd, nil
}
