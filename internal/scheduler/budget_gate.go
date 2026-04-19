package scheduler

import (
	"fmt"
	"time"
)

// BudgetLimits is the configured ceiling for daily/weekly task counts.
type BudgetLimits struct {
	DailyMax     int
	WeeklyMax    int
	WeekStartsOn time.Weekday
	ResetTZ      *time.Location
}

// BudgetCounters holds the persisted task counts for the current day/week buckets.
type BudgetCounters struct {
	DailyCount  int
	DailyKey    string // e.g. "2026-04-19"
	WeeklyCount int
	WeeklyKey   string // e.g. "2026-W17"
}

// RateLimitBlock represents an observed CLI rate-limit signal.
// Zero value means no active block.
type RateLimitBlock struct {
	BlockedUntil  time.Time // zero if none
	RateLimitType string
}

// Active reports whether the block is still in effect at now.
func (b RateLimitBlock) Active(now time.Time) bool {
	return !b.BlockedUntil.IsZero() && now.Before(b.BlockedUntil)
}

// BudgetReason explains a budget gate decision. Empty string means "allowed".
type BudgetReason string

// BudgetReasonAllowed and siblings enumerate budget gate decisions.
const (
	BudgetReasonAllowed     BudgetReason = ""
	BudgetReasonDailyCap    BudgetReason = "daily_cap_reached"
	BudgetReasonWeeklyCap   BudgetReason = "weekly_cap_reached"
	BudgetReasonRateLimited BudgetReason = "rate_limited"
)

// EvaluateBudget reports whether a new task may start at now.
// Caller is expected to have already performed bucket rollover (RolloverCounters).
// Limits with zero or negative max values are treated as "no limit".
func EvaluateBudget(now time.Time, counters BudgetCounters, limits BudgetLimits, block RateLimitBlock) BudgetReason {
	if block.Active(now) {
		return BudgetReasonRateLimited
	}
	if limits.DailyMax > 0 && counters.DailyCount >= limits.DailyMax {
		return BudgetReasonDailyCap
	}
	if limits.WeeklyMax > 0 && counters.WeeklyCount >= limits.WeeklyMax {
		return BudgetReasonWeeklyCap
	}
	return BudgetReasonAllowed
}

// DateKey returns "YYYY-MM-DD" in the limits TZ for use as the daily counter bucket.
func DateKey(now time.Time, tz *time.Location) string {
	if tz == nil {
		tz = time.UTC
	}
	return now.In(tz).Format("2006-01-02")
}

// WeekKey returns "YYYY-Www" identifying the week bucket (TZ-local), where the
// week starts on the configured weekday. The result is monotonic within a year
// and stable across DST shifts because it is derived from the local date only.
func WeekKey(now time.Time, tz *time.Location, weekStartsOn time.Weekday) string {
	if tz == nil {
		tz = time.UTC
	}
	local := now.In(tz)
	// Snap to the most recent week start.
	offset := int(local.Weekday()-weekStartsOn+7) % 7
	weekStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, tz).
		AddDate(0, 0, -offset)
	year, week := weekStart.ISOWeek()
	// ISOWeek anchors on Monday; for non-Monday week starts the displayed
	// year/week may diverge from ISO. We still get a unique-per-week key.
	return fmt.Sprintf("%04d-W%02d", year, week)
}

// RolloverCounters returns counters reset to zero when the day/week key has
// changed relative to now. If the bucket is still current the counters are
// returned unchanged (with empty keys filled in).
func RolloverCounters(now time.Time, counters BudgetCounters, limits BudgetLimits) BudgetCounters {
	dailyKey := DateKey(now, limits.ResetTZ)
	weeklyKey := WeekKey(now, limits.ResetTZ, limits.WeekStartsOn)

	out := counters
	if out.DailyKey != dailyKey {
		out.DailyKey = dailyKey
		out.DailyCount = 0
	}
	if out.WeeklyKey != weeklyKey {
		out.WeeklyKey = weeklyKey
		out.WeeklyCount = 0
	}
	return out
}
