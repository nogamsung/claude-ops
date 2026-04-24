package main

import "time"

// healthResponse mirrors api.HealthResponse for JSON decoding.
type healthResponse struct {
	Status   string     `json:"status"`
	TickAt   *time.Time `json:"tick_at,omitempty"`
	FullMode bool       `json:"full_mode"`
}

// taskResponse mirrors api.TaskResponse.
type taskResponse struct {
	ID                    string     `json:"id"`
	RepoFullName          string     `json:"repo_full_name"`
	IssueNumber           int        `json:"issue_number"`
	IssueTitle            string     `json:"issue_title"`
	TaskType              string     `json:"task_type"`
	Status                string     `json:"status"`
	PRURL                 string     `json:"pr_url,omitempty"`
	PRNumber              int        `json:"pr_number,omitempty"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	FinishedAt            *time.Time `json:"finished_at,omitempty"`
	EstimatedInputTokens  int        `json:"estimated_input_tokens,omitempty"`
	EstimatedOutputTokens int        `json:"estimated_output_tokens,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// taskEventResponse mirrors api.TaskEventResponse.
type taskEventResponse struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	PayloadJSON string    `json:"payload_json,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// taskDetailResponse mirrors api.TaskDetailResponse.
type taskDetailResponse struct {
	taskResponse
	StderrTail string              `json:"stderr_tail,omitempty"`
	Events     []taskEventResponse `json:"events,omitempty"`
}

// taskListResponse mirrors api.TaskListResponse.
type taskListResponse struct {
	Items      []taskResponse `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// enqueueRequest mirrors api.EnqueueRequest.
type enqueueRequest struct {
	RepoFullName string `json:"repo"`
	IssueNumber  int    `json:"issue_number"`
	IssueTitle   string `json:"issue_title,omitempty"`
}

// fullModeRequest mirrors api.FullModeRequest.
type fullModeRequest struct {
	Enabled bool `json:"enabled"`
}

// fullModeResponse mirrors api.FullModeResponse.
type fullModeResponse struct {
	Enabled bool       `json:"enabled"`
	Since   *time.Time `json:"since,omitempty"`
}

// limitsResponse mirrors api.LimitsResponse.
type limitsResponse struct {
	Daily     limitsDaily     `json:"daily"`
	Weekly    limitsWeekly    `json:"weekly"`
	RateLimit limitsRateLimit `json:"rate_limit"`
	Reason    string          `json:"reason,omitempty"`
}

// limitsDaily mirrors api.LimitsDaily.
type limitsDaily struct {
	Count int    `json:"count"`
	Max   int    `json:"max"`
	Date  string `json:"date"`
}

// limitsWeekly mirrors api.LimitsWeekly.
type limitsWeekly struct {
	Count int    `json:"count"`
	Max   int    `json:"max"`
	Week  string `json:"week"`
}

// limitsRateLimit mirrors api.LimitsRateLimit.
type limitsRateLimit struct {
	BlockedUntil  *time.Time `json:"blocked_until,omitempty"`
	RateLimitType string     `json:"rate_limit_type,omitempty"`
}

// limitsPatchRequest mirrors api.LimitsPatchRequest.
type limitsPatchRequest struct {
	DailyMax  *int `json:"daily_max,omitempty"`
	WeeklyMax *int `json:"weekly_max,omitempty"`
}
