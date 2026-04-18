// Package stream parses Claude CLI NDJSON stream-json output.
package stream

import "encoding/json"

// Envelope is the top-level wrapper for every NDJSON line from the Claude CLI.
type Envelope struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	UUID      string          `json:"uuid"`
	Raw       json.RawMessage `json:"-"` // preserved for unknown-type logging
}

// SystemInit is emitted once when Claude initialises the session.
type SystemInit struct {
	CWD            string `json:"cwd"`
	Model          string `json:"model"`
	PermissionMode string `json:"permissionMode"`
	// APIKeySource must be "none" for a subscription (Claude login) session.
	APIKeySource  string `json:"apiKeySource"`
	ClaudeCodeVer string `json:"claude_code_version"`
	OutputStyle   string `json:"output_style"`
}

// AssistantEvent is emitted for each assistant turn.
type AssistantEvent struct {
	Message struct {
		Model   string `json:"model"`
		ID      string `json:"id"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"` // "text" | "tool_use" | ...
			Text string `json:"text,omitempty"`
		} `json:"content"`
		Usage MessageUsage `json:"usage"`
	} `json:"message"`
}

// MessageUsage holds per-turn token counts.
type MessageUsage struct {
	InputTokens              int    `json:"input_tokens"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
	OutputTokens             int    `json:"output_tokens"`
	ServiceTier              string `json:"service_tier"`
}

// RateLimitEvent is emitted when the API rate-limit gate is triggered.
type RateLimitEvent struct {
	RateLimitInfo struct {
		Status                string `json:"status"`        // "allowed" | "blocked" | ...
		ResetsAt              int64  `json:"resetsAt"`      // unix seconds
		RateLimitType         string `json:"rateLimitType"` // "five_hour"
		OverageStatus         string `json:"overageStatus"`
		OverageDisabledReason string `json:"overageDisabledReason,omitempty"`
		IsUsingOverage        bool   `json:"isUsingOverage"`
	} `json:"rate_limit_info"`
}

// ResultEvent is emitted exactly once when Claude finishes.
type ResultEvent struct {
	Subtype           string                `json:"subtype"` // "success" | "error" | "error_during_execution"
	IsError           bool                  `json:"is_error"`
	APIErrorStatus    *string               `json:"api_error_status"`
	DurationMS        int64                 `json:"duration_ms"`
	DurationAPIMS     int64                 `json:"duration_api_ms"`
	NumTurns          int                   `json:"num_turns"`
	Result            string                `json:"result"`
	StopReason        string                `json:"stop_reason"` // "end_turn" | ...
	TotalCostUSD      float64               `json:"total_cost_usd"`
	Usage             ResultUsage           `json:"usage"`
	ModelUsage        map[string]ModelUsage `json:"modelUsage"`
	PermissionDenials []PermissionDenial    `json:"permission_denials"`
	TerminalReason    string                `json:"terminal_reason"` // "completed" | "interrupted" | ...
}

// ResultUsage holds aggregate token usage from the result event.
type ResultUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// ModelUsage holds per-model usage statistics.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
}

// PermissionDenial records a permission denial event.
type PermissionDenial struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
}
