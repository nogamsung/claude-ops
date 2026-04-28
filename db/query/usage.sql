-- name: SumUsageByDay :many
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
WHERE COALESCE(finished_at, created_at) >= sqlc.arg('from')
  AND COALESCE(finished_at, created_at) <  sqlc.arg('to')
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByWeek :many
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
WHERE COALESCE(finished_at, created_at) >= sqlc.arg('from')
  AND COALESCE(finished_at, created_at) <  sqlc.arg('to')
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByMonth :many
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
WHERE COALESCE(finished_at, created_at) >= sqlc.arg('from')
  AND COALESCE(finished_at, created_at) <  sqlc.arg('to')
GROUP BY bucket
ORDER BY bucket;

-- name: SumUsageByModel :many
SELECT
    cost_usd,
    model_usage_json,
    total_input_tokens,
    total_output_tokens,
    cache_read_input_tokens,
    cache_creation_input_tokens
FROM tasks
WHERE status = 'done'
  AND COALESCE(finished_at, created_at) >= sqlc.arg('from')
  AND COALESCE(finished_at, created_at) <  sqlc.arg('to');

-- name: SumDailyCost :one
SELECT COALESCE(SUM(cost_usd), 0) AS total_cost
FROM tasks
WHERE status = 'done'
  AND date(COALESCE(finished_at, created_at)) = sqlc.arg('day_key');

-- name: SumWeeklyCost :one
SELECT COALESCE(SUM(cost_usd), 0) AS total_cost
FROM tasks
WHERE status = 'done'
  AND strftime('%Y-W%W', COALESCE(finished_at, created_at)) = sqlc.arg('week_key');
