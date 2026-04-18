// Package scheduler implements the active-window gate and tick-based task dispatcher.
package scheduler

import "time"

// Clock abstracts time.Now() to enable fake time in tests.
type Clock interface {
	Now() time.Time
}

// RealClock returns the real system time.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock holds a fixed time for deterministic testing.
type FakeClock struct {
	T time.Time
}

// Now returns the fixed time.
func (f *FakeClock) Now() time.Time { return f.T }
