package scheduler_test

import (
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

func makeWindow(t *testing.T, days []string, start, end, tz string) *domain.ActiveWindow {
	t.Helper()
	w := &domain.ActiveWindow{Days: days, Start: start, End: end, TZ: tz}
	if err := w.Validate(); err != nil {
		t.Fatalf("validate window: %v", err)
	}
	return w
}

func TestAllowNow_FullModeBypassesWindow(t *testing.T) {
	// Even with no windows and full mode on, should allow.
	saturday := time.Date(2026, 4, 18, 3, 0, 0, 0, time.UTC)
	if !scheduler.AllowNow(saturday, true, nil) {
		t.Error("full mode should bypass window check")
	}
}

func TestAllowNow_InsideWindow(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon", "tue", "wed", "thu", "fri"}, "09:00", "18:00", "Asia/Seoul")

	// Monday 14:00 Seoul
	monday := time.Date(2026, 4, 20, 14, 0, 0, 0, seoulLoc)
	if !scheduler.AllowNow(monday, false, []*domain.ActiveWindow{w}) {
		t.Error("expected inside window to be allowed")
	}
}

func TestAllowNow_OutsideWindow(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon", "tue", "wed", "thu", "fri"}, "09:00", "18:00", "Asia/Seoul")

	// Saturday 14:00 Seoul
	saturday := time.Date(2026, 4, 18, 14, 0, 0, 0, seoulLoc)
	if scheduler.AllowNow(saturday, false, []*domain.ActiveWindow{w}) {
		t.Error("expected Saturday to be outside window")
	}
}

func TestAllowNow_MultipleWindows_FirstMatches(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w1 := makeWindow(t, []string{"mon"}, "09:00", "12:00", "Asia/Seoul")
	w2 := makeWindow(t, []string{"mon"}, "13:00", "18:00", "Asia/Seoul")

	// Monday 10:00 — inside w1
	t1 := time.Date(2026, 4, 20, 10, 0, 0, 0, seoulLoc)
	if !scheduler.AllowNow(t1, false, []*domain.ActiveWindow{w1, w2}) {
		t.Error("expected 10:00 to be inside w1")
	}

	// Monday 12:30 — between windows
	t2 := time.Date(2026, 4, 20, 12, 30, 0, 0, seoulLoc)
	if scheduler.AllowNow(t2, false, []*domain.ActiveWindow{w1, w2}) {
		t.Error("expected 12:30 to be between windows (disallowed)")
	}

	// Monday 14:00 — inside w2
	t3 := time.Date(2026, 4, 20, 14, 0, 0, 0, seoulLoc)
	if !scheduler.AllowNow(t3, false, []*domain.ActiveWindow{w1, w2}) {
		t.Error("expected 14:00 to be inside w2")
	}
}

func TestAllowNow_NoWindows_FullModeOff(t *testing.T) {
	// No windows configured, full mode off — never allowed.
	if scheduler.AllowNow(time.Now(), false, nil) {
		t.Error("expected empty windows to be disallowed")
	}
}

func TestAllowNow_TZBoundary(t *testing.T) {
	// UTC 00:00 = Asia/Seoul 09:00
	w := makeWindow(t, []string{"mon"}, "09:00", "18:00", "Asia/Seoul")
	utcMon := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC) // Monday UTC 00:00 = Seoul 09:00
	if !scheduler.AllowNow(utcMon, false, []*domain.ActiveWindow{w}) {
		t.Error("UTC 00:00 should map to Seoul 09:00 Monday (inside window)")
	}
}
