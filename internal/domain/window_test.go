package domain_test

import (
	"testing"
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

func makeWindow(t *testing.T, days []string, start, end, tz string) *domain.ActiveWindow {
	t.Helper()
	w := &domain.ActiveWindow{
		Days:  days,
		Start: start,
		End:   end,
		TZ:    tz,
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	return w
}

func TestActiveWindow_Contains_Weekday(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon", "tue", "wed", "thu", "fri"}, "09:00", "18:00", "Asia/Seoul")

	// Monday 10:00 Seoul — inside
	monday := time.Date(2026, 4, 20, 10, 0, 0, 0, seoulLoc) // Monday
	if !w.Contains(monday) {
		t.Error("expected Monday 10:00 Seoul to be inside window")
	}

	// Saturday — outside
	saturday := time.Date(2026, 4, 18, 10, 0, 0, 0, seoulLoc) // Saturday
	if w.Contains(saturday) {
		t.Error("expected Saturday to be outside window")
	}
}

func TestActiveWindow_Contains_Boundaries(t *testing.T) {
	seoulLoc, _ := time.LoadLocation("Asia/Seoul")
	w := makeWindow(t, []string{"mon"}, "09:00", "18:00", "Asia/Seoul")

	// Exact start — inside
	atStart := time.Date(2026, 4, 20, 9, 0, 0, 0, seoulLoc)
	if !w.Contains(atStart) {
		t.Error("expected 09:00 to be inside (inclusive)")
	}

	// Exact end — outside (exclusive)
	atEnd := time.Date(2026, 4, 20, 18, 0, 0, 0, seoulLoc)
	if w.Contains(atEnd) {
		t.Error("expected 18:00 to be outside (exclusive)")
	}

	// One minute before start — outside
	beforeStart := time.Date(2026, 4, 20, 8, 59, 0, 0, seoulLoc)
	if w.Contains(beforeStart) {
		t.Error("expected 08:59 to be outside")
	}

	// One minute before end — inside
	beforeEnd := time.Date(2026, 4, 20, 17, 59, 0, 0, seoulLoc)
	if !w.Contains(beforeEnd) {
		t.Error("expected 17:59 to be inside")
	}
}

func TestActiveWindow_Contains_TZConversion(t *testing.T) {
	// UTC time 01:00 = Asia/Seoul 10:00 (+09:00)
	w := makeWindow(t, []string{"mon"}, "09:00", "18:00", "Asia/Seoul")
	utcTime := time.Date(2026, 4, 20, 1, 0, 0, 0, time.UTC)
	if !w.Contains(utcTime) {
		t.Error("expected UTC 01:00 (Seoul 10:00 Monday) to be inside window")
	}
}

func TestActiveWindow_Validate_Errors(t *testing.T) {
	tests := []struct {
		name  string
		w     domain.ActiveWindow
		errOk bool
	}{
		{"invalid TZ", domain.ActiveWindow{Days: []string{"mon"}, Start: "09:00", End: "18:00", TZ: "BadZone"}, true},
		{"invalid start", domain.ActiveWindow{Days: []string{"mon"}, Start: "25:00", End: "18:00", TZ: "UTC"}, true},
		{"invalid end", domain.ActiveWindow{Days: []string{"mon"}, Start: "09:00", End: "09:00", TZ: "UTC"}, true},
		{"end before start", domain.ActiveWindow{Days: []string{"mon"}, Start: "18:00", End: "09:00", TZ: "UTC"}, true},
		{"invalid day", domain.ActiveWindow{Days: []string{"xyz"}, Start: "09:00", End: "18:00", TZ: "UTC"}, true},
		{"no days", domain.ActiveWindow{Days: []string{}, Start: "09:00", End: "18:00", TZ: "UTC"}, true},
		{"valid", domain.ActiveWindow{Days: []string{"mon"}, Start: "09:00", End: "18:00", TZ: "UTC"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.w.Validate()
			if tc.errOk && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.errOk && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
