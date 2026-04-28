package domain

import "errors"

// ErrNotFound is returned when a requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrOutsideActiveWindow is returned when a task is requested outside the active time window.
var ErrOutsideActiveWindow = errors.New("outside active window")

// ErrAlreadyRunning is returned when a task for the same issue is already running.
var ErrAlreadyRunning = errors.New("task already running for this issue")

// ErrFullModeOff is returned when a full-mode-only action is attempted with full mode disabled.
var ErrFullModeOff = errors.New("full mode is off")

// ErrClaudeUsageExhausted is returned when the Claude CLI signals usage exhaustion.
var ErrClaudeUsageExhausted = errors.New("claude usage exhausted")

// ErrSessionMissing is returned when the Claude CLI session is not logged in.
var ErrSessionMissing = errors.New("claude session missing: please run 'claude login' on this machine")

// ErrTaskNotCancellable is returned when a task cannot be cancelled in its current state.
var ErrTaskNotCancellable = errors.New("task cannot be cancelled in its current state")

// ErrInvalidRange is returned when the from date is after the to date.
var ErrInvalidRange = errors.New("from date must not be after to date")

// ErrRangeTooLarge is returned when the requested date range exceeds 365 days.
var ErrRangeTooLarge = errors.New("date range must not exceed 365 days")

// ErrInvalidBucket is returned when an unsupported group_by value is provided.
var ErrInvalidBucket = errors.New("group_by must be one of: day, week, month")
