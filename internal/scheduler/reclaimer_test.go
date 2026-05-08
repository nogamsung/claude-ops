package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gs97ahn/claude-ops/internal/domain"
	"github.com/gs97ahn/claude-ops/internal/scheduler"
)

// reclaimSpy records every ReclaimStale call and returns the configured n/err.
// Other TaskRepository methods are no-ops because the Reclaimer never calls
// them.
type reclaimSpy struct {
	calls   atomic.Int32
	cutoffs []time.Time
	n       int64
	err     error
}

func (s *reclaimSpy) ReclaimStale(_ context.Context, cutoff time.Time) (int64, error) {
	s.calls.Add(1)
	s.cutoffs = append(s.cutoffs, cutoff)
	return s.n, s.err
}

// Stub the rest of the interface — the Reclaimer never invokes them.
func (s *reclaimSpy) Create(context.Context, *domain.Task) error            { return nil }
func (s *reclaimSpy) GetByID(context.Context, string) (*domain.Task, error) { return nil, nil }
func (s *reclaimSpy) Update(context.Context, *domain.Task) error            { return nil }
func (s *reclaimSpy) List(context.Context, domain.TaskFilter) ([]*domain.Task, error) {
	return nil, nil
}
func (s *reclaimSpy) GetRunning(context.Context) ([]*domain.Task, error) { return nil, nil }
func (s *reclaimSpy) ExistsByRepoAndIssue(context.Context, string, int) (bool, error) {
	return false, nil
}
func (s *reclaimSpy) ClaimNext(context.Context, string) (*domain.Task, error) {
	return nil, nil
}
func (s *reclaimSpy) ListWatchingIDs(context.Context) ([]string, error) { return nil, nil }
func (s *reclaimSpy) FindFixChild(context.Context, string, string) (*domain.Task, error) {
	return nil, nil
}
func (s *reclaimSpy) UpdateCIStatus(context.Context, string, domain.CIStatus, string, time.Time) error {
	return nil
}

func TestReclaimer_TicksUntilCancel(t *testing.T) {
	t.Parallel()

	spy := &reclaimSpy{n: 2}
	r := scheduler.NewReclaimer(spy, scheduler.RealClock{}, 10*time.Millisecond, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r.Start(ctx)

	// At 10ms cadence over ~100ms we expect at least 5 ticks; lower bound is
	// loose because CI scheduling jitter on slow runners can shave one off.
	if got := spy.calls.Load(); got < 3 {
		t.Errorf("expected at least 3 reclaim calls, got %d", got)
	}
}

func TestReclaimer_CutoffEqualsNowMinusLease(t *testing.T) {
	t.Parallel()

	lease := 30 * time.Minute
	spy := &reclaimSpy{}
	r := scheduler.NewReclaimer(spy, scheduler.RealClock{}, 5*time.Millisecond, lease)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	r.Start(ctx)

	if len(spy.cutoffs) == 0 {
		t.Fatal("expected at least one ReclaimStale call")
	}
	delta := time.Since(spy.cutoffs[0])
	// cutoff was set as `now - lease` at the tick, so wall delta should be ≈ lease (within 1s).
	if delta < lease-time.Second || delta > lease+time.Second {
		t.Errorf("cutoff age = %s, expected ≈ lease (%s)", delta, lease)
	}
}

func TestReclaimer_DisabledWhenMisconfigured(t *testing.T) {
	t.Parallel()

	spy := &reclaimSpy{}

	// Zero interval → disabled, returns immediately.
	r := scheduler.NewReclaimer(spy, scheduler.RealClock{}, 0, time.Hour)
	r.Start(context.Background())
	if got := spy.calls.Load(); got != 0 {
		t.Errorf("expected 0 calls when disabled, got %d", got)
	}
}
