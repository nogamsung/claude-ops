package github

import (
	"context"
	"sync"
	"time"
)

// DedupCache prevents double-processing the same webhook delivery.
type DedupCache interface {
	// CheckAndAdd returns true (accepted) if deliveryID is new, false (duplicate) if already seen.
	CheckAndAdd(deliveryID string) bool
}

// Clock abstracts time.Now() for testable TTL logic.
type Clock interface {
	Now() time.Time
}

// realClock returns real system time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// memDedupCache is an in-memory DedupCache backed by sync.Map with TTL eviction.
type memDedupCache struct {
	items sync.Map // map[string]time.Time (expiresAt)
	ttl   time.Duration
	clock Clock
}

// NewMemDedupCache creates a MemDedupCache that evicts entries after ttl.
// A background GC goroutine runs every 60s and stops when ctx is cancelled.
func NewMemDedupCache(ctx context.Context, ttl time.Duration) DedupCache {
	return NewMemDedupCacheWithClock(ctx, ttl, realClock{})
}

// NewMemDedupCacheWithClock creates a MemDedupCache with an injected clock (for testing).
func NewMemDedupCacheWithClock(ctx context.Context, ttl time.Duration, clk Clock) *memDedupCache {
	c := &memDedupCache{ttl: ttl, clock: clk}
	go c.gcLoop(ctx)
	return c
}

// CheckAndAdd returns true if deliveryID is new (and records it), false if it is a duplicate.
func (c *memDedupCache) CheckAndAdd(deliveryID string) bool {
	now := c.clock.Now()
	expiresAt := now.Add(c.ttl)

	// LoadOrStore is atomic: if key already exists, return the existing value.
	existing, loaded := c.items.LoadOrStore(deliveryID, expiresAt)
	if !loaded {
		// First time seen.
		return true
	}

	// Key exists; check whether the existing entry has expired.
	if exp, ok := existing.(time.Time); ok && now.After(exp) {
		// Entry is expired — overwrite and accept.
		c.items.Store(deliveryID, expiresAt)
		return true
	}

	// Within TTL — it's a duplicate.
	return false
}

// gcLoop removes expired entries every 60 seconds until ctx is done.
func (c *memDedupCache) gcLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.evictExpired()
		}
	}
}

func (c *memDedupCache) evictExpired() {
	now := c.clock.Now()
	c.items.Range(func(key, value any) bool {
		if exp, ok := value.(time.Time); ok && now.After(exp) {
			c.items.Delete(key)
		}
		return true
	})
}

// EvictExpiredForTesting exposes evictExpired for white-box testing of GC behaviour.
func (c *memDedupCache) EvictExpiredForTesting() {
	c.evictExpired()
}
