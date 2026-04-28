// Package domain contains pure business entities with no external dependencies.
package domain

import "time"

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

// TaskStatusQueued, TaskStatusRunning, TaskStatusDone, TaskStatusFailed,
// TaskStatusCancelled, and TaskStatusOrphaned enumerate the task lifecycle
// states. Orphaned means: the service restarted while the task was running
// and we found a dirty worktree — needs human judgment before we decide
// whether to retry or discard.
const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusDone      TaskStatus = "done"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusOrphaned  TaskStatus = "orphaned"
)

// TaskType represents the category of work a task performs.
type TaskType string

// TaskTypeFeature, TaskTypeSecurity, and TaskTypePerf enumerate the kinds of
// work a task can perform.
const (
	TaskTypeFeature  TaskType = "feature"
	TaskTypeSecurity TaskType = "security"
	TaskTypePerf     TaskType = "perf"
)

// TaskSource identifies how a task was created.
type TaskSource string

// TaskSourceGitHubIssue and TaskSourceMaintenance enumerate task origin types.
const (
	TaskSourceGitHubIssue TaskSource = "github_issue"
	TaskSourceMaintenance TaskSource = "maintenance"
)

// Task is the central aggregate of the scheduler system.
type Task struct {
	ID                       string
	RepoFullName             string
	IssueNumber              int
	IssueTitle               string
	TaskType                 TaskType
	Status                   TaskStatus
	Source                   TaskSource
	MaintenanceName          string
	PromptTemplate           string
	WorktreePath             string
	PRURL                    string
	PRNumber                 int
	StartedAt                *time.Time
	FinishedAt               *time.Time
	EstimatedInputTokens     int
	EstimatedOutputTokens    int
	ExitCode                 *int
	StderrTail               string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	// Cost and usage fields populated from the Claude CLI result event.
	CostUSD                  float64
	TotalInputTokens         int64
	TotalOutputTokens        int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	ModelUsageJSON           string // raw JSON; empty value is "{}"
}

// EventKind is the type of a task lifecycle event.
type EventKind string

// EventKindStarted and its siblings enumerate the kinds of task lifecycle
// events recorded by the scheduler.
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
