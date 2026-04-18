// Package domain contains pure business entities with no external dependencies.
package domain

import "time"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusDone      TaskStatus = "done"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// TaskType represents the category of work a task performs.
type TaskType string

const (
	TaskTypeFeature  TaskType = "feature"
	TaskTypeSecurity TaskType = "security"
	TaskTypePerf     TaskType = "perf"
)

// Task is the central aggregate of the scheduler system.
type Task struct {
	ID                     string
	RepoFullName           string
	IssueNumber            int
	IssueTitle             string
	TaskType               TaskType
	Status                 TaskStatus
	PromptTemplate         string
	WorktreePath           string
	PRURL                  string
	PRNumber               int
	StartedAt              *time.Time
	FinishedAt             *time.Time
	EstimatedInputTokens   int
	EstimatedOutputTokens  int
	ExitCode               *int
	StderrTail             string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// EventKind is the type of a task lifecycle event.
type EventKind string

const (
	EventKindStarted           EventKind = "started"
	EventKindSlackSent         EventKind = "slack_sent"
	EventKindClaudeStdoutChunk EventKind = "claude_stdout_chunk"
	EventKindCancelled         EventKind = "cancelled"
	EventKindUsageWarning      EventKind = "usage_warning"
	EventKindPRCreated         EventKind = "pr_created"
	EventKindFailed            EventKind = "failed"
)

// TaskEvent records a single event in a task's lifecycle.
type TaskEvent struct {
	ID          string
	TaskID      string
	Kind        EventKind
	PayloadJSON string
	CreatedAt   time.Time
}

// AppState is a key-value singleton for persisting application state.
type AppState struct {
	Key       string
	ValueJSON string
	UpdatedAt time.Time
}
