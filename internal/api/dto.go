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
	Source                string     `json:"source" example:"github_issue"`
	MaintenanceName       string     `json:"maintenance_name,omitempty" example:"daily-dep-update"`
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

// WebhookResponse is the response for POST /github/webhook.
type WebhookResponse struct {
	Accepted bool   `json:"accepted" example:"true"`
	Reason   string `json:"reason,omitempty" example:"queued"`
	TaskID   string `json:"task_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
}

// UsageBucketResponse is a single time-bucket in a usage aggregation response.
type UsageBucketResponse struct {
	Bucket              string  `json:"bucket" example:"2026-04-27"`
	TaskCount           int64   `json:"task_count" example:"4"`
	CostUSD             float64 `json:"cost_usd" example:"1.23"`
	InputTokens         int64   `json:"input_tokens" example:"12345"`
	OutputTokens        int64   `json:"output_tokens" example:"6789"`
	CacheReadTokens     int64   `json:"cache_read_tokens" example:"100"`
	CacheCreationTokens int64   `json:"cache_creation_tokens" example:"50"`
	FailedCostUSD       float64 `json:"failed_cost_usd" example:"0.0"`
}

// UsageTotalsResponse summarises the totals across all returned buckets.
type UsageTotalsResponse struct {
	TaskCount           int64   `json:"task_count" example:"80"`
	CostUSD             float64 `json:"cost_usd" example:"24.50"`
	InputTokens         int64   `json:"input_tokens" example:"200000"`
	OutputTokens        int64   `json:"output_tokens" example:"80000"`
	CacheReadTokens     int64   `json:"cache_read_tokens" example:"5000"`
	CacheCreationTokens int64   `json:"cache_creation_tokens" example:"2000"`
	FailedCostUSD       float64 `json:"failed_cost_usd" example:"0.40"`
}

// UsageResponse is the response for GET /usage.
type UsageResponse struct {
	From    string                `json:"from" example:"2026-03-28"`
	To      string                `json:"to" example:"2026-04-27"`
	GroupBy string                `json:"group_by" example:"day"`
	Buckets []UsageBucketResponse `json:"buckets"`
	Totals  UsageTotalsResponse   `json:"totals"`
}

// UsageModelItemResponse is a single model in a by-model usage response.
type UsageModelItemResponse struct {
	ModelID             string  `json:"model_id" example:"claude-sonnet-4-5"`
	TaskCount           int64   `json:"task_count" example:"60"`
	CostUSD             float64 `json:"cost_usd" example:"18.20"`
	InputTokens         int64   `json:"input_tokens" example:"150000"`
	OutputTokens        int64   `json:"output_tokens" example:"60000"`
	CacheReadTokens     int64   `json:"cache_read_tokens" example:"4000"`
	CacheCreationTokens int64   `json:"cache_creation_tokens" example:"1500"`
}

// UsageByModelResponse is the response for GET /usage/by-model.
type UsageByModelResponse struct {
	From   string                   `json:"from" example:"2026-03-28"`
	To     string                   `json:"to" example:"2026-04-27"`
	Models []UsageModelItemResponse `json:"models"`
}

// UsageLimitsPeriod holds cost usage vs limit for a single period (daily or weekly).
type UsageLimitsPeriod struct {
	CountUSD float64  `json:"count_usd" example:"0.85"`
	MaxUSD   float64  `json:"max_usd" example:"1.00"`
	Percent  *float64 `json:"percent" example:"85.0"`
}

// UsageLimitsDailyResponse holds the daily period data.
type UsageLimitsDailyResponse struct {
	UsageLimitsPeriod
	Date string `json:"date" example:"2026-04-27"`
}

// UsageLimitsWeeklyResponse holds the weekly period data.
type UsageLimitsWeeklyResponse struct {
	UsageLimitsPeriod
	Week string `json:"week" example:"2026-W17"`
}

// UsageLimitsResponse is the response for GET /usage/limits.
type UsageLimitsResponse struct {
	Daily  UsageLimitsDailyResponse  `json:"daily"`
	Weekly UsageLimitsWeeklyResponse `json:"weekly"`
}
