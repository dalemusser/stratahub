package mhsactivity

import (
	"sync"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// dayCache keeps a live day's computation for a short while and makes
// concurrent requests for the same zone-day share one computation.
type dayCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	done chan struct{}
	at   time.Time
	docs []models.MHSActivityDay
	err  error
}

func newDayCache(ttl time.Duration) *dayCache {
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &dayCache{ttl: ttl, max: 200, entries: map[string]*cacheEntry{}}
}

// Get returns the cached result for key, or computes it (once, even when
// several callers ask at the same time). The returned time is when the
// result was computed.
func (c *dayCache) Get(key string, now time.Time, compute func() ([]models.MHSActivityDay, error)) ([]models.MHSActivityDay, time.Time, error) {
	c.mu.Lock()
	e := c.entries[key]
	if e != nil {
		select {
		case <-e.done:
			if e.err == nil && now.Sub(e.at) < c.ttl {
				c.mu.Unlock()
				return e.docs, e.at, nil
			}
			e = nil // stale, or a failure worth retrying
		default:
			// still computing: wait for it below
		}
	}
	if e == nil {
		c.evict(now)
		e = &cacheEntry{done: make(chan struct{})}
		c.entries[key] = e
		c.mu.Unlock()
		e.docs, e.err = compute()
		e.at = time.Now()
		close(e.done)
		return e.docs, e.at, e.err
	}
	c.mu.Unlock()
	<-e.done
	return e.docs, e.at, e.err
}

// evict drops expired entries, and the oldest ones past the cap. Call with
// the lock held.
func (c *dayCache) evict(now time.Time) {
	for k, e := range c.entries {
		select {
		case <-e.done:
			if e.err != nil || now.Sub(e.at) >= c.ttl {
				delete(c.entries, k)
			}
		default:
		}
	}
	for len(c.entries) >= c.max {
		var oldestKey string
		var oldest time.Time
		for k, e := range c.entries {
			select {
			case <-e.done:
				if oldestKey == "" || e.at.Before(oldest) {
					oldestKey, oldest = k, e.at
				}
			default:
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.entries, oldestKey)
	}
}
