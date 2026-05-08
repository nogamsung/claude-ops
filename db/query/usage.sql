-- name: SumUsageByDay :many
SELECT
    DATE_FORMAT(COALESCE(finished_at, created_at), '%Y-%m-%d')                                                AS bucket,
    CAST(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END) AS SIGNED)                                          AS task_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cost_usd ELSE 0 END), 0) AS DOUBLE)                      AS cost_usd,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_input_tokens ELSE 0 END), 0) AS SIGNED)            AS input_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_output_tokens ELSE 0 END), 0) AS SIGNED)           AS output_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_read_input_tokens ELSE 0 END), 0) AS SIGNED)       AS cache_read_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_creation_input_tokens ELSE 0 END), 0) AS SIGNED)   AS cache_creation_tokens,
    CAST(COALESCE(SUM(CASE WHEN status IN ('failed', 'cancelled') THEN cost_usd ELSE 0 END), 0) AS DOUBLE)    AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= sqlc.arg(from_ts)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(to_ts)
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByWeek :many
-- ISO 8601 week (year-week, Monday start, week 1 contains Jan 4).
SELECT
    DATE_FORMAT(COALESCE(finished_at, created_at), '%x-W%v')                                                  AS bucket,
    CAST(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END) AS SIGNED)                                          AS task_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cost_usd ELSE 0 END), 0) AS DOUBLE)                      AS cost_usd,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_input_tokens ELSE 0 END), 0) AS SIGNED)            AS input_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_output_tokens ELSE 0 END), 0) AS SIGNED)           AS output_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_read_input_tokens ELSE 0 END), 0) AS SIGNED)       AS cache_read_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_creation_input_tokens ELSE 0 END), 0) AS SIGNED)   AS cache_creation_tokens,
    CAST(COALESCE(SUM(CASE WHEN status IN ('failed', 'cancelled') THEN cost_usd ELSE 0 END), 0) AS DOUBLE)    AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= sqlc.arg(from_ts)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(to_ts)
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByMonth :many
SELECT
    DATE_FORMAT(COALESCE(finished_at, created_at), '%Y-%m')                                                   AS bucket,
    CAST(SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END) AS SIGNED)                                          AS task_count,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cost_usd ELSE 0 END), 0) AS DOUBLE)                      AS cost_usd,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_input_tokens ELSE 0 END), 0) AS SIGNED)            AS input_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN total_output_tokens ELSE 0 END), 0) AS SIGNED)           AS output_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_read_input_tokens ELSE 0 END), 0) AS SIGNED)       AS cache_read_tokens,
    CAST(COALESCE(SUM(CASE WHEN status = 'done' THEN cache_creation_input_tokens ELSE 0 END), 0) AS SIGNED)   AS cache_creation_tokens,
    CAST(COALESCE(SUM(CASE WHEN status IN ('failed', 'cancelled') THEN cost_usd ELSE 0 END), 0) AS DOUBLE)    AS failed_cost_usd
FROM tasks
WHERE COALESCE(finished_at, created_at) >= sqlc.arg(from_ts)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(to_ts)
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByModel :many
SELECT
    CAST(cost_usd AS DOUBLE)                                                                                  AS cost_usd,
    model_usage_json,
    total_input_tokens,
    total_output_tokens,
    cache_read_input_tokens,
    cache_creation_input_tokens
FROM tasks
WHERE status = 'done'
  AND COALESCE(finished_at, created_at) >= sqlc.arg(from_ts)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(to_ts);

-- name: SumDailyCost :one
-- Caller resolves the day boundary in the configured tz and passes
-- [day_start, day_end) as a half-open range (e.g. 00:00:00.000 of day N
-- and day N+1). Range comparison lets MySQL use the timestamp index.
SELECT CAST(COALESCE(SUM(cost_usd), 0) AS DOUBLE) AS total_cost
FROM tasks
WHERE status = 'done'
  AND COALESCE(finished_at, created_at) >= sqlc.arg(day_start)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(day_end);

-- name: SumWeeklyCost :one
-- Caller resolves the ISO 8601 week boundary in the configured tz and passes
-- [week_start, week_end) as a half-open range (Monday 00:00 of week N and N+1).
SELECT CAST(COALESCE(SUM(cost_usd), 0) AS DOUBLE) AS total_cost
FROM tasks
WHERE status = 'done'
  AND COALESCE(finished_at, created_at) >= sqlc.arg(week_start)
  AND COALESCE(finished_at, created_at) <  sqlc.arg(week_end);
