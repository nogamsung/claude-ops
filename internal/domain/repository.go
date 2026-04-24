package domain

import "context"

// TaskFilter contains optional filters for listing tasks.
type TaskFilter struct {
	Status *TaskStatus
	Source *TaskSource
	Limit  int
	Cursor string // opaque cursor (task ID for keyset pagination)
}

// TaskRepository defines storage operations for Task entities.
type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	GetByID(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, task *Task) error
	List(ctx context.Context, filter TaskFilter) ([]*Task, error)
	GetRunning(ctx context.Context) ([]*Task, error)
	ExistsByRepoAndIssue(ctx context.Context, repoFullName string, issueNumber int) (bool, error)
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
