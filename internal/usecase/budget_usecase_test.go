package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/scheduler"
	"github.com/gs97ahn/claude-ops/internal/usecase"
)

func defaultLimits(t *testing.T) scheduler.BudgetLimits {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("load tz: %v", err)
	}
	return scheduler.BudgetLimits{
		DailyMax:     2,
		WeeklyMax:    5,
		WeekStartsOn: time.Monday,
		ResetTZ:      loc,
	}
}

func TestBudgetUseCase_FreshSnapshot_Allows(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))
	now := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)

	snap, err := uc.Snapshot(context.Background(), now)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Reason != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed, got %q", snap.Reason)
	}
	if snap.Counters.DailyCount != 0 || snap.Counters.WeeklyCount != 0 {
		t.Errorf("expected fresh counters, got %+v", snap.Counters)
	}
}

func TestBudgetUseCase_CheckAndIncrement_HitsDailyCap(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))
	now := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		reason, _, err := uc.CheckAndIncrement(ctx, now)
		if err != nil {
			t.Fatalf("inc %d: %v", i, err)
		}
		if reason != scheduler.BudgetReasonAllowed {
			t.Fatalf("inc %d: expected allowed, got %q", i, reason)
		}
	}

	reason, snap, err := uc.CheckAndIncrement(ctx, now)
	if err != nil {
		t.Fatalf("inc 3: %v", err)
	}
	if reason != scheduler.BudgetReasonDailyCap {
		t.Errorf("expected daily_cap_reached, got %q", reason)
	}
	if snap.Counters.DailyCount != 2 {
		t.Errorf("expected daily count 2 (not incremented), got %d", snap.Counters.DailyCount)
	}
}

func TestBudgetUseCase_RolloverNextDay(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))
	ctx := context.Background()

	day1 := time.Date(2026, 4, 20, 23, 0, 0, 0, time.UTC) // Mon Seoul = Tue 08:00
	for i := 0; i < 2; i++ {
		if _, _, err := uc.CheckAndIncrement(ctx, day1); err != nil {
			t.Fatalf("day1 inc %d: %v", i, err)
		}
	}
	// Tomorrow same week — daily resets, weekly does not.
	day2 := day1.Add(24 * time.Hour)
	reason, snap, err := uc.CheckAndIncrement(ctx, day2)
	if err != nil {
		t.Fatalf("day2: %v", err)
	}
	if reason != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed after daily rollover, got %q", reason)
	}
	if snap.Counters.DailyCount != 1 {
		t.Errorf("expected daily count 1 after rollover, got %d", snap.Counters.DailyCount)
	}
	if snap.Counters.WeeklyCount != 3 {
		t.Errorf("expected weekly count 3 (preserved), got %d", snap.Counters.WeeklyCount)
	}
}

func TestBudgetUseCase_WeeklyCap(t *testing.T) {
	repo := &fakeAppStateRepo{}
	limits := defaultLimits(t)
	limits.DailyMax = 0 // unlimited daily
	limits.WeeklyMax = 3
	uc := usecase.NewBudgetUseCase(repo, limits)
	ctx := context.Background()

	now := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, _, err := uc.CheckAndIncrement(ctx, now); err != nil {
			t.Fatalf("inc %d: %v", i, err)
		}
	}
	reason, _, err := uc.CheckAndIncrement(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if reason != scheduler.BudgetReasonWeeklyCap {
		t.Errorf("expected weekly_cap_reached, got %q", reason)
	}
}

func TestBudgetUseCase_RateLimitBlock_PreventsAndExpires(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))
	ctx := context.Background()

	now := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	resetsAt := now.Add(2 * time.Hour)
	if err := uc.RecordRateLimitBlock(ctx, resetsAt.Unix(), "five_hour", now); err != nil {
		t.Fatalf("record block: %v", err)
	}

	reason, _, err := uc.CheckAndIncrement(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if reason != scheduler.BudgetReasonRateLimited {
		t.Errorf("expected rate_limited, got %q", reason)
	}

	// After the block expires, gate opens again.
	later := resetsAt.Add(time.Minute)
	reason, _, err = uc.CheckAndIncrement(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if reason != scheduler.BudgetReasonAllowed {
		t.Errorf("expected allowed after expiry, got %q", reason)
	}
}

func TestBudgetUseCase_SetLimitsOverride(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))
	ctx := context.Background()

	eff, err := uc.SetLimits(ctx, 10, 20)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if eff.DailyMax != 10 || eff.WeeklyMax != 20 {
		t.Errorf("expected daily=10 weekly=20, got %+v", eff)
	}

	// Snapshot should pick up the override.
	snap, err := uc.Snapshot(ctx, time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Limits.DailyMax != 10 || snap.Limits.WeeklyMax != 20 {
		t.Errorf("snapshot did not pick up override: %+v", snap.Limits)
	}
}

func TestBudgetUseCase_SetLimits_RejectsInvalid(t *testing.T) {
	repo := &fakeAppStateRepo{}
	uc := usecase.NewBudgetUseCase(repo, defaultLimits(t))

	if _, err := uc.SetLimits(context.Background(), -1, 5); err == nil {
		t.Error("expected error for negative daily")
	}
	if _, err := uc.SetLimits(context.Background(), 10, 5); err == nil {
		t.Error("expected error for daily > weekly")
	}
}
