package scheduler

import (
	"time"

	"github.com/gs97ahn/scheduled-dev-agent/internal/domain"
)

// AllowNow reports whether a task may execute at now given the active windows and full-mode flag.
// When fullMode is true, the window check is bypassed.
// Pure function — no side effects.
func AllowNow(now time.Time, fullMode bool, windows []*domain.ActiveWindow) bool {
	if fullMode {
		return true
	}
	for _, w := range windows {
		if w.Contains(now) {
			return true
		}
	}
	return false
}
