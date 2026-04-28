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

// memDedupCache is an in-memory DedupCache backed by a mutex-protected map with TTL eviction. // MODIFIED
type memDedupCache struct {
	mu    sync.Mutex           // MODIFIED: replaced sync.Map with mutex + plain map
	items map[string]time.Time // MODIFIED: deliveryID → expiresAt
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
	c := &memDedupCache{ttl: ttl, clock: clk, items: make(map[string]time.Time)} // MODIFIED
	go c.gcLoop(ctx)
	return c
}

// CheckAndAdd returns true if deliveryID is new (and records it), false if it is a duplicate.
// The entire check-and-store is performed under the mutex to eliminate the TOCTOU race
// that existed with the previous LoadOrStore → TTL check → Store pattern. // MODIFIED
func (c *memDedupCache) CheckAndAdd(deliveryID string) bool {
	c.mu.Lock()         // MODIFIED
	defer c.mu.Unlock() // MODIFIED
	now := c.clock.Now()
	if exp, ok := c.items[deliveryID]; ok && now.Before(exp) { // MODIFIED
		// Within TTL — it's a duplicate.
		return false
	}
	// First time seen, or TTL has expired — accept and record.
	c.items[deliveryID] = now.Add(c.ttl) // MODIFIED
	return true
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
	c.mu.Lock()         // MODIFIED
	defer c.mu.Unlock() // MODIFIED
	now := c.clock.Now()
	for key, exp := range c.items { // MODIFIED
		if now.After(exp) {
			delete(c.items, key) // MODIFIED
		}
	}
}

// EvictExpiredForTesting exposes evictExpired for white-box testing of GC behaviour.
func (c *memDedupCache) EvictExpiredForTesting() {
	c.evictExpired()
}
