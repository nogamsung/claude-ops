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

// TaskTypeFeature, TaskTypeSecurity, TaskTypePerf, and TaskTypeCIFix
// enumerate the kinds of work a task can perform. TaskTypeCIFix is a
// follow-up auto-spawned by the CI watcher when a PR's checks fail.
const (
	TaskTypeFeature  TaskType = "feature"
	TaskTypeSecurity TaskType = "security"
	TaskTypePerf     TaskType = "perf"
	TaskTypeCIFix    TaskType = "ci-fix"
)

// CIStatus tracks the CI watcher state for a task. Empty means "not
// watched" (legacy row, ci-fix disabled, or PR creation never succeeded).
type CIStatus string

// CIStatus enum.
const (
	CIStatusEmpty     CIStatus = ""
	CIStatusPending   CIStatus = "pending"
	CIStatusWatching  CIStatus = "watching"
	CIStatusPassed    CIStatus = "passed"
	CIStatusFailed    CIStatus = "failed"
	CIStatusExhausted CIStatus = "exhausted"
	CIStatusTimeout   CIStatus = "timeout"
	CIStatusStuck     CIStatus = "stuck"
	CIStatusClosed    CIStatus = "closed"
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
	ID                    string
	RepoFullName          string
	IssueNumber           int
	IssueTitle            string
	TaskType              TaskType
	Status                TaskStatus
	Source                TaskSource
	MaintenanceName       string
	PromptTemplate        string
	WorktreePath          string
	PRURL                 string
	PRNumber              int
	StartedAt             *time.Time
	FinishedAt            *time.Time
	EstimatedInputTokens  int
	EstimatedOutputTokens int
	ExitCode              *int
	StderrTail            string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	// Cost and usage fields populated from the Claude CLI result event.
	CostUSD                  float64
	TotalInputTokens         int64
	TotalOutputTokens        int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	ModelUsageJSON           string // raw JSON; empty value is "{}"
	// Concurrency claim state (parallel-tasks). WorkerID is the opaque token
	// of the worker that holds the row; ClaimedAt is when ClaimNextTask
	// flipped it to running. Both are nil for queued/terminal rows.
	WorkerID  *string
	ClaimedAt *time.Time

	// CI-fix loop state (ci-fix-loop). ParentTaskID forms the parent→child
	// fix-chain; FixAttemptCount is incremented per chain link and capped by
	// config.CIFixConfig.MaxAttempts. CIStatus tracks the watcher lifecycle
	// for the PR this task created. HeadSHA + (ParentTaskID, HeadSHA) is the
	// dedup key for fix tasks. CILastPolledAt is informational.
	ParentTaskID    *string
	FixAttemptCount int
	CIStatus        CIStatus
	HeadSHA         string
	CILastPolledAt  *time.Time
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
	// ci-fix-loop lifecycle events.
	EventKindCICheckPolled     EventKind = "ci_check_polled"
	EventKindCIFailureDetected EventKind = "ci_failure_detected"
	EventKindCIFixEnqueued     EventKind = "ci_fix_enqueued"
	EventKindCIFixExhausted    EventKind = "ci_fix_exhausted"
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
