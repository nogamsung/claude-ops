// Hand-written sqlc-style query implementations for usage aggregation.
// TODO: replace with `sqlc generate` output once sqlc CLI is available.
// Keep the same Querier interface signatures so the generated file
// can drop in without touching call sites.

package sqlcdb

import (
	"context"
	"time"
)

// SumUsageByDayParams holds arguments for SumUsageByDay.
type SumUsageByDayParams struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// SumUsageByDayRow is a single row returned by SumUsageByDay.
type SumUsageByDayRow struct {
	Bucket              string  `json:"bucket"`
	TaskCount           int64   `json:"task_count"`
	CostUsd             float64 `json:"cost_usd"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	FailedCostUsd       float64 `json:"failed_cost_usd"`
}

// SumUsageByWeekParams holds arguments for SumUsageByWeek.
type SumUsageByWeekParams struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// SumUsageByWeekRow is a single row returned by SumUsageByWeek.
type SumUsageByWeekRow = SumUsageByDayRow

// SumUsageByMonthParams holds arguments for SumUsageByMonth.
type SumUsageByMonthParams struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// SumUsageByMonthRow is a single row returned by SumUsageByMonth.
type SumUsageByMonthRow = SumUsageByDayRow

// SumUsageByModelParams holds arguments for SumUsageByModel.
type SumUsageByModelParams struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// SumUsageByModelRow is a single row returned by SumUsageByModel.
type SumUsageByModelRow struct {
	CostUsd                  float64 `json:"cost_usd"`
	ModelUsageJson           string  `json:"model_usage_json"`
	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	CacheReadInputTokens     int64   `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64   `json:"cache_creation_input_tokens"`
}

// SumDailyCostParams holds arguments for SumDailyCost.
type SumDailyCostParams struct {
	DayKey string `json:"day_key"`
}

// SumWeeklyCostParams holds arguments for SumWeeklyCost.
type SumWeeklyCostParams struct {
	WeekKey string `json:"week_key"`
}

const sumUsageByDay = `-- name: SumUsageByDay :many
SELECT
    date(COALESCE(finished_at, created_at))                                                  AS bucket,
    COUNT(*) FILTER (WHERE status = 'done')                                                  AS task_count,
    COALESCE(SUM(cost_usd) FILTER (WHERE status = 'done'), 0)                               AS cost_usd,
    COALESCE(SUM(total_input_tokens) FILTER (WHERE status = 'done'), 0)                     AS input_tokens,
    COALESCE(SUM(total_output_tokens) FILTER (WHERE status = 'done'), 0)                    AS output_tokens,
    COALESCE(SUM(cache_read_input_tokens) FILTER (WHERE status = 'done'), 0)                AS cache_read_tokens,
    COALESCE(SUM(cache_creation_input_tokens) FILTER (WHERE status = 'done'), 0)            AS cache_creation_tokens,
    COALESCE(SUM(cost_usd) FILTER (WHERE status IN ('failed', 'cancelled')), 0)             AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= ?
  AND COALESCE(finished_at, created_at) <  ?
GROUP BY bucket
ORDER BY bucket`

const sumUsageByWeek = `-- name: SumUsageByWeek :many
SELECT
    strftime('%Y-W%W', COALESCE(finished_at, created_at))                                   AS bucket,
    COUNT(*) FILTER (WHERE status = 'done')                                                  AS task_count,
    COALESCE(SUM(cost_usd) FILTER (WHERE status = 'done'), 0)                               AS cost_usd,
    COALESCE(SUM(total_input_tokens) FILTER (WHERE status = 'done'), 0)                     AS input_tokens,
    COALESCE(SUM(total_output_tokens) FILTER (WHERE status = 'done'), 0)                    AS output_tokens,
    COALESCE(SUM(cache_read_input_tokens) FILTER (WHERE status = 'done'), 0)                AS cache_read_tokens,
    COALESCE(SUM(cache_creation_input_tokens) FILTER (WHERE status = 'done'), 0)            AS cache_creation_tokens,
    COALESCE(SUM(cost_usd) FILTER (WHERE status IN ('failed', 'cancelled')), 0)             AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= ?
  AND COALESCE(finished_at, created_at) <  ?
GROUP BY bucket
ORDER BY bucket`

const sumUsageByMonth = `-- name: SumUsageByMonth :many
SELECT
    strftime('%Y-%m', COALESCE(finished_at, created_at))                                    AS bucket,
    COUNT(*) FILTER (WHERE status = 'done')                                                  AS task_count,
    COALESCE(SUM(cost_usd) FILTER (WHERE status = 'done'), 0)                               AS cost_usd,
    COALESCE(SUM(total_input_tokens) FILTER (WHERE status = 'done'), 0)                     AS input_tokens,
    COALESCE(SUM(total_output_tokens) FILTER (WHERE status = 'done'), 0)                    AS output_tokens,
    COALESCE(SUM(cache_read_input_tokens) FILTER (WHERE status = 'done'), 0)                AS cache_read_tokens,
    COALESCE(SUM(cache_creation_input_tokens) FILTER (WHERE status = 'done'), 0)            AS cache_creation_tokens,
    COALESCE(SUM(cost_usd) FILTER (WHERE status IN ('failed', 'cancelled')), 0)             AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= ?
  AND COALESCE(finished_at, created_at) <  ?
GROUP BY bucket
ORDER BY bucket`

const sumUsageByModel = `-- name: SumUsageByModel :many
SELECT
    cost_usd,
    model_usage_json,
    total_input_tokens,
    total_output_tokens,
    cache_read_input_tokens,
    cache_creation_input_tokens
FROM tasks
WHERE status = 'done'
  AND COALESCE(finished_at, created_at) >= ?
  AND COALESCE(finished_at, created_at) <  ?`

const sumDailyCost = `-- name: SumDailyCost :one
SELECT COALESCE(SUM(cost_usd), 0) AS total_cost
FROM tasks
WHERE status = 'done'
  AND date(COALESCE(finished_at, created_at)) = ?`

const sumWeeklyCost = `-- name: SumWeeklyCost :one
SELECT COALESCE(SUM(cost_usd), 0) AS total_cost
FROM tasks
WHERE status = 'done'
  AND strftime('%Y-W%W', COALESCE(finished_at, created_at)) = ?`

func scanUsageBucketRow(rows interface{ Scan(...interface{}) error }) (SumUsageByDayRow, error) {
	var r SumUsageByDayRow
	err := rows.Scan(
		&r.Bucket,
		&r.TaskCount,
		&r.CostUsd,
		&r.InputTokens,
		&r.OutputTokens,
		&r.CacheReadTokens,
		&r.CacheCreationTokens,
		&r.FailedCostUsd,
	)
	return r, err
}

// SumUsageByDay executes the SumUsageByDay query.
func (q *Queries) SumUsageByDay(ctx context.Context, arg SumUsageByDayParams) ([]SumUsageByDayRow, error) {
	rows, err := q.db.QueryContext(ctx, sumUsageByDay, arg.From, arg.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SumUsageByDayRow
	for rows.Next() {
		r, err := scanUsageBucketRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// SumUsageByWeek executes the SumUsageByWeek query.
func (q *Queries) SumUsageByWeek(ctx context.Context, arg SumUsageByWeekParams) ([]SumUsageByDayRow, error) {
	rows, err := q.db.QueryContext(ctx, sumUsageByWeek, arg.From, arg.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SumUsageByDayRow
	for rows.Next() {
		r, err := scanUsageBucketRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// SumUsageByMonth executes the SumUsageByMonth query.
func (q *Queries) SumUsageByMonth(ctx context.Context, arg SumUsageByMonthParams) ([]SumUsageByDayRow, error) {
	rows, err := q.db.QueryContext(ctx, sumUsageByMonth, arg.From, arg.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SumUsageByDayRow
	for rows.Next() {
		r, err := scanUsageBucketRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// SumUsageByModel executes the SumUsageByModel query.
func (q *Queries) SumUsageByModel(ctx context.Context, arg SumUsageByModelParams) ([]SumUsageByModelRow, error) {
	rows, err := q.db.QueryContext(ctx, sumUsageByModel, arg.From, arg.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SumUsageByModelRow
	for rows.Next() {
		var r SumUsageByModelRow
		if err := rows.Scan(
			&r.CostUsd,
			&r.ModelUsageJson,
			&r.TotalInputTokens,
			&r.TotalOutputTokens,
			&r.CacheReadInputTokens,
			&r.CacheCreationInputTokens,
		); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// SumDailyCost executes the SumDailyCost query.
func (q *Queries) SumDailyCost(ctx context.Context, dayKey string) (float64, error) {
	var total float64
	err := q.db.QueryRowContext(ctx, sumDailyCost, dayKey).Scan(&total)
	return total, err
}

// SumWeeklyCost executes the SumWeeklyCost query.
func (q *Queries) SumWeeklyCost(ctx context.Context, weekKey string) (float64, error) {
	var total float64
	err := q.db.QueryRowContext(ctx, sumWeeklyCost, weekKey).Scan(&total)
	return total, err
}
