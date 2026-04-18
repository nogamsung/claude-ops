-- name: ListTasks :many
SELECT id, repo_full_name, issue_number, issue_title, task_type, status,
       prompt_template, worktree_path, pr_url, pr_number,
       started_at, finished_at,
       estimated_input_tokens, estimated_output_tokens,
       exit_code, stderr_tail, created_at, updated_at
FROM tasks
WHERE (sqlc.narg('status') IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('cursor') IS NULL OR id < sqlc.narg('cursor'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: CountTasksByStatus :many
SELECT status, COUNT(*) as count
FROM tasks
GROUP BY status;
