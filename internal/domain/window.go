package domain

import (
	"fmt"
	"strings"
	"time"
)

// Weekday maps lowercase day abbreviation to time.Weekday.
var weekdayMap = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

// ActiveWindow defines a recurring time range during which tasks are permitted.
type ActiveWindow struct {
	Days  []string // e.g. ["mon","tue","wed","thu","fri"]
	Start string   // "HH:MM" in 24-hour format
	End   string   // "HH:MM" in 24-hour format
	TZ    string   // IANA timezone, e.g. "Asia/Seoul"

	// Parsed fields (populated by Validate).
	weekdays map[time.Weekday]struct{}
	startH   int
	startM   int
	endH     int
	endM     int
	loc      *time.Location
}

// Validate parses and validates the window fields, returning an error on failure.
func (w *ActiveWindow) Validate() error {
	loc, err := time.LoadLocation(w.TZ)
	if err != nil {
		return fmt.Errorf("invalid timezone %q: %w", w.TZ, err)
	}
	w.loc = loc

	if err = parseHHMM(w.Start, &w.startH, &w.startM); err != nil {
		return fmt.Errorf("invalid start time %q: %w", w.Start, err)
	}
	if err = parseHHMM(w.End, &w.endH, &w.endM); err != nil {
		return fmt.Errorf("invalid end time %q: %w", w.End, err)
	}

	startMinutes := w.startH*60 + w.startM
	endMinutes := w.endH*60 + w.endM
	if endMinutes <= startMinutes {
		return fmt.Errorf("end time %q must be after start time %q", w.End, w.Start)
	}

	w.weekdays = make(map[time.Weekday]struct{}, len(w.Days))
	for _, d := range w.Days {
		wd, ok := weekdayMap[strings.ToLower(d)]
		if !ok {
			return fmt.Errorf("invalid day abbreviation %q", d)
		}
		w.weekdays[wd] = struct{}{}
	}
	if len(w.weekdays) == 0 {
		return fmt.Errorf("at least one day must be specified")
	}

	return nil
}

// Contains reports whether t falls within this active window.
// Validate must have been called successfully before Contains.
func (w *ActiveWindow) Contains(t time.Time) bool {
	local := t.In(w.loc)

	if _, ok := w.weekdays[local.Weekday()]; !ok {
		return false
	}

	totalMin := local.Hour()*60 + local.Minute()
	startMin := w.startH*60 + w.startM
	endMin := w.endH*60 + w.endM

	return totalMin >= startMin && totalMin < endMin
}

func parseHHMM(s string, h, m *int) error {
	var hour, min int
	if _, err := fmt.Sscanf(s, "%d:%d", &hour, &min); err != nil {
		return fmt.Errorf("expected HH:MM format: %w", err)
	}
	if hour < 0 || hour > 23 {
		return fmt.Errorf("hour %d out of range [0,23]", hour)
	}
	if min < 0 || min > 59 {
		return fmt.Errorf("minute %d out of range [0,59]", min)
	}
	*h = hour
	*m = min
	return nil
}
