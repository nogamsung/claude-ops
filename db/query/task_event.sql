-- name: ListEventsByTask :many
SELECT id, task_id, kind, payload_json, created_at
FROM task_events
WHERE task_id = sqlc.arg('task_id')
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');
