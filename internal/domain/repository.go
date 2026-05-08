package domain

import (
	"context"
	"time"
)

// TaskFilter contains optional filters for listing tasks.
type TaskFilter struct {
	Status   *TaskStatus
	Source   *TaskSource
	CIStatus *CIStatus
	Limit    int
	Cursor   string // opaque cursor (task ID for keyset pagination)
}

// TaskRepository defines storage operations for Task entities.
type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	GetByID(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, task *Task) error
	List(ctx context.Context, filter TaskFilter) ([]*Task, error)
	GetRunning(ctx context.Context) ([]*Task, error)
	ExistsByRepoAndIssue(ctx context.Context, repoFullName string, issueNumber int) (bool, error)

	// ClaimNext atomically picks the oldest queued task whose repo has no
	// running task yet, sets status=running with the given workerID, and
	// returns the loaded Task. Returns (nil, nil) when no eligible row exists.
	// Implementations rely on FOR UPDATE SKIP LOCKED so multiple workers can
	// call this concurrently without coordinating in-process.
	ClaimNext(ctx context.Context, workerID string) (*Task, error)

	// ReclaimStale marks running tasks whose claim is older than `cutoff`
	// (typically now - lease_timeout) as orphaned and clears the worker_id.
	// Returns the number of rows reclaimed. Called periodically by the
	// scheduler to recover after a crashed worker.
	ReclaimStale(ctx context.Context, cutoff time.Time) (int64, error)

	// ListWatchingIDs returns task IDs whose CI watcher is active. Sorted
	// least-recently-polled first so the watcher loop processes oldest first.
	ListWatchingIDs(ctx context.Context) ([]string, error)

	// FindFixChild returns the existing ci-fix child task for a (parent,
	// head_sha) dedup key, or (nil, nil) when none exists. Used by the
	// watcher to decide whether enqueueing a fix would be a duplicate.
	FindFixChild(ctx context.Context, parentTaskID, headSHA string) (*Task, error)

	// UpdateCIStatus is a targeted update used by the watcher to advance
	// ci_status / head_sha / ci_last_polled_at without rewriting the row.
	UpdateCIStatus(ctx context.Context, id string, status CIStatus, headSHA string, polledAt time.Time) error
}

// TaskEventRepository defines storage operations for TaskEvent entities.
type TaskEventRepository interface {
	Create(ctx context.Context, event *TaskEvent) error
	ListByTaskID(ctx context.Context, taskID string, limit int) ([]*TaskEvent, error)
}

// AppStateRepository defines key-value singleton storage for AppState.
type AppStateRepository interface {
	Get(ctx context.Context, key string) (*AppState, error)
	Set(ctx context.Context, state *AppState) error
}
