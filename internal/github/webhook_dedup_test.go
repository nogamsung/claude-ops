package github_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	igithub "github.com/gs97ahn/claude-ops/internal/github"
)

// fakeClock is a Clock implementation whose current time is settable.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestDedupCache_NewDelivery_Accepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, newFakeClock(time.Now()))
	if !cache.CheckAndAdd("delivery-1") {
		t.Error("first delivery should be accepted")
	}
}

func TestDedupCache_DuplicateWithinTTL_Rejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clk := newFakeClock(time.Now())
	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, clk)

	if !cache.CheckAndAdd("delivery-2") {
		t.Fatal("first call must be accepted")
	}
	// Advance 1 minute — still within 5-minute TTL.
	clk.Advance(1 * time.Minute)
	if cache.CheckAndAdd("delivery-2") {
		t.Error("duplicate within TTL should be rejected")
	}
}

func TestDedupCache_AfterTTL_Accepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clk := newFakeClock(time.Now())
	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, clk)

	if !cache.CheckAndAdd("delivery-3") {
		t.Fatal("first call must be accepted")
	}
	// Advance past 5-minute TTL.
	clk.Advance(5*time.Minute + 1*time.Second)
	if !cache.CheckAndAdd("delivery-3") {
		t.Error("same ID after TTL expiry should be accepted again")
	}
}

// TestDedupCache_Concurrent_ExactlyOneAccepted verifies that under concurrent load,
// exactly one goroutine wins the CheckAndAdd race for the same delivery ID.
func TestDedupCache_Concurrent_ExactlyOneAccepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, newFakeClock(time.Now()))

	const goroutines = 1000
	var accepted atomic.Int64

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if cache.CheckAndAdd("shared-delivery") {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if accepted.Load() != 1 {
		t.Errorf("expected exactly 1 acceptance, got %d", accepted.Load())
	}
}

func TestDedupCache_DifferentIDs_BothAccepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, newFakeClock(time.Now()))

	if !cache.CheckAndAdd("delivery-A") {
		t.Error("delivery-A should be accepted")
	}
	if !cache.CheckAndAdd("delivery-B") {
		t.Error("delivery-B should be accepted")
	}
}

// TestDedupCache_NewMemDedupCache_WithRealClock tests the public constructor
// that uses the real system clock, ensuring it can be created and used.
func TestDedupCache_NewMemDedupCache_WithRealClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := igithub.NewMemDedupCache(ctx, 5*time.Minute)
	if !cache.CheckAndAdd("real-clock-delivery") {
		t.Error("first delivery should be accepted")
	}
	if cache.CheckAndAdd("real-clock-delivery") {
		t.Error("duplicate within TTL should be rejected")
	}
}

// TestDedupCache_TTLBoundary_ExactlyOneAccepted verifies that when the TTL has just expired
// and 100 goroutines simultaneously call CheckAndAdd with the same delivery ID,
// exactly one goroutine wins the race and gets true.
func TestDedupCache_TTLBoundary_ExactlyOneAccepted(t *testing.T) { // ADDED
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clk := newFakeClock(time.Now())
	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, clk)

	// t=0: register the delivery ID.
	if !cache.CheckAndAdd("boundary-delivery") {
		t.Fatal("first call must be accepted")
	}

	// Advance past TTL so the entry is expired.
	clk.Advance(5*time.Minute + 1*time.Millisecond)

	// 100 goroutines race to CheckAndAdd the same expired ID — exactly 1 must win.
	const goroutines = 100
	var accepted atomic.Int64

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if cache.CheckAndAdd("boundary-delivery") {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	if accepted.Load() != 1 {
		t.Errorf("TTL boundary: expected exactly 1 acceptance, got %d", accepted.Load())
	}
}

// TestDedupCache_EvictExpired exercises the GC eviction path directly.
func TestDedupCache_EvictExpired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clk := newFakeClock(time.Now())
	cache := igithub.NewMemDedupCacheWithClock(ctx, 5*time.Minute, clk)

	if !cache.CheckAndAdd("evict-target") {
		t.Fatal("first call must be accepted")
	}
	// Advance past TTL.
	clk.Advance(5*time.Minute + 1*time.Second)

	// Trigger GC directly.
	cache.EvictExpiredForTesting()

	// After eviction, the entry should be gone and accepted again (not via TTL bypass).
	if !cache.CheckAndAdd("evict-target") {
		t.Error("evicted entry should be accepted after GC removes it")
	}
}
