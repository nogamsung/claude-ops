package api

import "time"

// HealthResponse is the response for GET /healthz.
type HealthResponse struct {
	Status   string     `json:"status" example:"ok"`
	TickAt   *time.Time `json:"tick_at,omitempty" example:"2026-04-18T09:00:00Z"`
	FullMode bool       `json:"full_mode" example:"false"`
}

// TaskResponse is the summary response for a single task.
type TaskResponse struct {
	ID                    string     `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	RepoFullName          string     `json:"repo_full_name" example:"owner/repo"`
	IssueNumber           int        `json:"issue_number" example:"42"`
	IssueTitle            string     `json:"issue_title" example:"Fix the bug"`
	TaskType              string     `json:"task_type" example:"feature"`
	Status                string     `json:"status" example:"queued"`
	PRURL                 string     `json:"pr_url,omitempty" example:"https://github.com/owner/repo/pull/10"`
	PRNumber              int        `json:"pr_number,omitempty" example:"10"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	FinishedAt            *time.Time `json:"finished_at,omitempty"`
	EstimatedInputTokens  int        `json:"estimated_input_tokens,omitempty" example:"1000"`
	EstimatedOutputTokens int        `json:"estimated_output_tokens,omitempty" example:"500"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// TaskDetailResponse includes task details plus recent events.
type TaskDetailResponse struct {
	TaskResponse
	StderrTail string              `json:"stderr_tail,omitempty"`
	Events     []TaskEventResponse `json:"events,omitempty"`
}

// TaskEventResponse is a single task lifecycle event.
type TaskEventResponse struct {
	ID          string    `json:"id" example:"event-uuid"`
	Kind        string    `json:"kind" example:"started"`
	PayloadJSON string    `json:"payload_json,omitempty" example:"{}"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskListResponse wraps a list of tasks with pagination cursor.
type TaskListResponse struct {
	Items      []TaskResponse `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty" example:"550e8400-e29b-41d4-a716-446655440001"`
}

// EnqueueRequest is the request body for POST /tasks.
type EnqueueRequest struct {
	RepoFullName string `json:"repo" binding:"required" example:"owner/repo"`
	IssueNumber  int    `json:"issue_number" binding:"required,min=1" example:"42"`
	IssueTitle   string `json:"issue_title,omitempty" example:"Fix the bug"`
}

// StopResponse is the response for POST /tasks/{id}/stop.
type StopResponse struct {
	Accepted bool `json:"accepted" example:"true"`
}

// FullModeRequest is the request body for POST /modes/full.
type FullModeRequest struct {
	Enabled bool `json:"enabled" example:"true"`
}

// FullModeResponse is the response for GET/POST /modes/full.
type FullModeResponse struct {
	Enabled bool       `json:"enabled" example:"true"`
	Since   *time.Time `json:"since,omitempty"`
}

// ErrorResponse is a generic error response.
type ErrorResponse struct {
	Error string `json:"error" example:"not found"`
}

// LimitsResponse is the GET /modes/limits response.
type LimitsResponse struct {
	Daily     LimitsDaily     `json:"daily"`
	Weekly    LimitsWeekly    `json:"weekly"`
	RateLimit LimitsRateLimit `json:"rate_limit"`
	Reason    string          `json:"reason,omitempty" example:"daily_cap_reached"`
}

// LimitsDaily summarises the daily-bucket budget.
type LimitsDaily struct {
	Count int    `json:"count" example:"3"`
	Max   int    `json:"max" example:"5"`
	Date  string `json:"date" example:"2026-04-19"`
}

// LimitsWeekly summarises the weekly-bucket budget.
type LimitsWeekly struct {
	Count int    `json:"count" example:"12"`
	Max   int    `json:"max" example:"35"`
	Week  string `json:"week" example:"2026-W17"`
}

// LimitsRateLimit reports the currently observed CLI rate-limit block.
type LimitsRateLimit struct {
	BlockedUntil  *time.Time `json:"blocked_until,omitempty"`
	RateLimitType string     `json:"rate_limit_type,omitempty" example:"five_hour"`
}

// LimitsPatchRequest is the PATCH /modes/limits request body.
// Either field may be omitted (or set to 0) to fall back to the config-derived value.
type LimitsPatchRequest struct {
	DailyMax  *int `json:"daily_max,omitempty" example:"5"`
	WeeklyMax *int `json:"weekly_max,omitempty" example:"35"`
}
