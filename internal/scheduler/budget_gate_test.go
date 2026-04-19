package scheduler_test

import (
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func TestEvaluateBudget_AllowedWhenUnderCaps(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 1, WeeklyCount: 3}
	l := scheduler.BudgetLimits{DailyMax: 5, WeeklyMax: 35}
	if got := scheduler.EvaluateBudget(now, c, l, scheduler.RateLimitBlock{}); got != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed, got %q", got)
	}
}

func TestEvaluateBudget_RateLimitedTakesPrecedence(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 0, WeeklyCount: 0}
	l := scheduler.BudgetLimits{DailyMax: 5, WeeklyMax: 35}
	block := scheduler.RateLimitBlock{
		BlockedUntil:  now.Add(2 * time.Hour),
		RateLimitType: "five_hour",
	}
	if got := scheduler.EvaluateBudget(now, c, l, block); got != scheduler.BudgetReasonRateLimited {
		t.Errorf("expected rate_limited, got %q", got)
	}
}

func TestEvaluateBudget_BlockExpired(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 0, WeeklyCount: 0}
	l := scheduler.BudgetLimits{DailyMax: 5, WeeklyMax: 35}
	block := scheduler.RateLimitBlock{
		BlockedUntil:  now.Add(-1 * time.Minute), // already passed
		RateLimitType: "five_hour",
	}
	if got := scheduler.EvaluateBudget(now, c, l, block); got != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed (block expired), got %q", got)
	}
}

func TestEvaluateBudget_DailyCapReached(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 5, WeeklyCount: 5}
	l := scheduler.BudgetLimits{DailyMax: 5, WeeklyMax: 35}
	if got := scheduler.EvaluateBudget(now, c, l, scheduler.RateLimitBlock{}); got != scheduler.BudgetReasonDailyCap {
		t.Errorf("expected daily_cap_reached, got %q", got)
	}
}

func TestEvaluateBudget_WeeklyCapReached(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 1, WeeklyCount: 35}
	l := scheduler.BudgetLimits{DailyMax: 5, WeeklyMax: 35}
	if got := scheduler.EvaluateBudget(now, c, l, scheduler.RateLimitBlock{}); got != scheduler.BudgetReasonWeeklyCap {
		t.Errorf("expected weekly_cap_reached, got %q", got)
	}
}

func TestEvaluateBudget_ZeroLimitMeansUnlimited(t *testing.T) {
	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	c := scheduler.BudgetCounters{DailyCount: 9999, WeeklyCount: 9999}
	l := scheduler.BudgetLimits{DailyMax: 0, WeeklyMax: 0}
	if got := scheduler.EvaluateBudget(now, c, l, scheduler.RateLimitBlock{}); got != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed for zero limits, got %q", got)
	}
}

func TestRolloverCounters_DailyKeyChange(t *testing.T) {
	seoul := mustLoc(t, "Asia/Seoul")
	limits := scheduler.BudgetLimits{
		DailyMax:     5,
		WeeklyMax:    35,
		WeekStartsOn: time.Monday,
		ResetTZ:      seoul,
	}
	prev := scheduler.BudgetCounters{
		DailyCount:  4,
		DailyKey:    "2026-04-18",
		WeeklyCount: 10,
		WeeklyKey:   "2026-W16",
	}
	now := time.Date(2026, 4, 19, 1, 0, 0, 0, seoul) // next day in Seoul
	out := scheduler.RolloverCounters(now, prev, limits)

	if out.DailyCount != 0 {
		t.Errorf("expected daily count reset, got %d", out.DailyCount)
	}
	if out.DailyKey != "2026-04-19" {
		t.Errorf("expected daily key 2026-04-19, got %q", out.DailyKey)
	}
	// 2026-04-19 is Sunday — same week (Mon..Sun) as 2026-04-13..04-19.
	// Weekly should NOT roll over when only the day changed within the same week.
	if out.WeeklyCount != 10 {
		t.Errorf("expected weekly count preserved, got %d", out.WeeklyCount)
	}
}

func TestRolloverCounters_WeeklyKeyChange(t *testing.T) {
	seoul := mustLoc(t, "Asia/Seoul")
	limits := scheduler.BudgetLimits{
		DailyMax:     5,
		WeeklyMax:    35,
		WeekStartsOn: time.Monday,
		ResetTZ:      seoul,
	}
	// Start: counters for previous week (Mon 2026-04-13 .. Sun 2026-04-19)
	prev := scheduler.BudgetCounters{
		DailyCount:  3,
		DailyKey:    "2026-04-19",
		WeeklyCount: 25,
		WeeklyKey:   scheduler.WeekKey(time.Date(2026, 4, 19, 12, 0, 0, 0, seoul), seoul, time.Monday),
	}
	// Now: Monday 2026-04-20 — new week begins
	now := time.Date(2026, 4, 20, 9, 0, 0, 0, seoul)
	out := scheduler.RolloverCounters(now, prev, limits)

	if out.DailyCount != 0 {
		t.Errorf("expected daily reset on new day, got %d", out.DailyCount)
	}
	if out.WeeklyCount != 0 {
		t.Errorf("expected weekly reset on new week, got %d", out.WeeklyCount)
	}
}

func TestRolloverCounters_NoChangeWithinSameBucket(t *testing.T) {
	seoul := mustLoc(t, "Asia/Seoul")
	limits := scheduler.BudgetLimits{
		DailyMax:     5,
		WeeklyMax:    35,
		WeekStartsOn: time.Monday,
		ResetTZ:      seoul,
	}
	dailyKey := "2026-04-20"
	weeklyKey := scheduler.WeekKey(time.Date(2026, 4, 20, 9, 0, 0, 0, seoul), seoul, time.Monday)
	prev := scheduler.BudgetCounters{
		DailyCount:  3,
		DailyKey:    dailyKey,
		WeeklyCount: 8,
		WeeklyKey:   weeklyKey,
	}
	now := time.Date(2026, 4, 20, 23, 30, 0, 0, seoul)
	out := scheduler.RolloverCounters(now, prev, limits)

	if out.DailyCount != 3 || out.WeeklyCount != 8 {
		t.Errorf("expected counters preserved, got daily=%d weekly=%d", out.DailyCount, out.WeeklyCount)
	}
}

func TestDateKey_TZBoundary(t *testing.T) {
	seoul := mustLoc(t, "Asia/Seoul")
	// UTC 14:30 = Seoul 23:30 (same day Apr 19)
	if got := scheduler.DateKey(time.Date(2026, 4, 19, 14, 30, 0, 0, time.UTC), seoul); got != "2026-04-19" {
		t.Errorf("expected 2026-04-19, got %q", got)
	}
	// UTC 15:01 = Seoul 00:01 next day (Apr 20)
	if got := scheduler.DateKey(time.Date(2026, 4, 19, 15, 1, 0, 0, time.UTC), seoul); got != "2026-04-20" {
		t.Errorf("expected 2026-04-20 (TZ rollover), got %q", got)
	}
}
