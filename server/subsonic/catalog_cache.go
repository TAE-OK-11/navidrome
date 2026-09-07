package subsonic

import (
	"context"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const catalogLoadTimeout = 30 * time.Second

type catalogEntry[V any] struct {
	value   V
	expires time.Time
}

// catalogCache stores immutable catalog snapshots. A generation prevents loads
// started before invalidation from publishing stale data afterwards.
// Expired entries are reclaimed on admission, keeping cache hits read-only.
type catalogCache[V any] struct {
	mu         sync.RWMutex
	entries    map[string]catalogEntry[V]
	generation uint64
	group      singleflight.Group
}

func (c *catalogCache[V]) get(key string, now time.Time) (V, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	return entry.value, ok && now.Before(entry.expires)
}

func (c *catalogCache[V]) store(key string, now time.Time, value V, limit int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeLocked(key, now, value, limit, ttl)
}

func (c *catalogCache[V]) storeLocked(key string, now time.Time, value V, limit int, ttl time.Duration) {
	if limit <= 0 {
		return
	}
	if c.entries == nil {
		c.entries = make(map[string]catalogEntry[V])
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= limit {
		var oldestKey string
		var oldestExpiry time.Time
		for candidate, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, candidate)
				continue
			}
			if oldestExpiry.IsZero() || entry.expires.Before(oldestExpiry) {
				oldestKey, oldestExpiry = candidate, entry.expires
			}
		}
		if len(c.entries) >= limit {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = catalogEntry[V]{value: value, expires: now.Add(ttl)}
}

func (c *catalogCache[V]) invalidate(matches func(string) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	for key := range c.entries {
		if matches == nil || matches(key) {
			delete(c.entries, key)
		}
	}
}

func (c *catalogCache[V]) load(ctx context.Context, key string, limit int, ttl time.Duration, loader func(context.Context) (V, error)) (V, error) {
	var zero V
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if value, ok := c.get(key, time.Now()); ok {
		return value, nil
	}
	c.mu.RLock()
	generation := c.generation
	c.mu.RUnlock()
	// New callers after invalidation must not join an obsolete in-flight load.
	flightKey := strconv.FormatUint(generation, 10) + ":" + key
	result := c.group.DoChan(flightKey, func() (any, error) {
		if value, ok := c.get(key, time.Now()); ok {
			return value, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), catalogLoadTimeout)
		defer cancel()
		value, err := loader(loadCtx)
		if err != nil {
			return zero, err
		}
		c.mu.Lock()
		if c.generation == generation {
			c.storeLocked(key, time.Now(), value, limit, ttl)
		}
		c.mu.Unlock()
		return value, nil
	})
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case result := <-result:
		if result.Err != nil {
			return zero, result.Err
		}
		return result.Val.(V), nil
	}
}
