package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"golang.org/x/sync/singleflight"
)

const (
	nativeAuthUserCacheLimit  = 256
	nativeAuthUserCacheTTL    = 10 * time.Second
	nativeAuthUserLoadTimeout = 3 * time.Second
)

type nativeAuthUserCacheEntry struct {
	user    *model.User
	expires time.Time
}

// nativeAuthUserCache briefly reuses loaded UI users so bursty /api traffic
// (including keepalive) does not pay FindByUsername on every request.
type nativeAuthUserCache struct {
	mu      sync.RWMutex
	entries map[string]nativeAuthUserCacheEntry
	group   singleflight.Group
	limit   int
	ttl     time.Duration
}

func newNativeAuthUserCache(limit int, ttl time.Duration) *nativeAuthUserCache {
	return &nativeAuthUserCache{entries: make(map[string]nativeAuthUserCacheEntry), limit: limit, ttl: ttl}
}

var nativeAuthUsers = newNativeAuthUserCache(nativeAuthUserCacheLimit, nativeAuthUserCacheTTL)

func (c *nativeAuthUserCache) get(ctx context.Context, key string, load func(context.Context) (*model.User, error)) (*model.User, error) {
	key = strings.ToLower(key)
	if user, ok := c.lookup(key, time.Now()); ok {
		return user, nil
	}
	result := c.group.DoChan(key, func() (any, error) {
		if user, ok := c.lookup(key, time.Now()); ok {
			return user, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nativeAuthUserLoadTimeout)
		defer cancel()
		user, err := load(loadCtx)
		if err != nil {
			return nil, err
		}
		value := *user
		c.store(key, &value, time.Now())
		return &value, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.(*model.User), nil
	}
}

func (c *nativeAuthUserCache) lookup(key string, now time.Time) (*model.User, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	return entry.user, ok && now.Before(entry.expires)
}

func (c *nativeAuthUserCache) store(key string, user *model.User, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.limit {
		for existingKey, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, existingKey)
			}
		}
		if len(c.entries) >= c.limit {
			for existingKey := range c.entries {
				delete(c.entries, existingKey)
				break
			}
		}
	}
	c.entries[key] = nativeAuthUserCacheEntry{user: user, expires: now.Add(c.ttl)}
}
