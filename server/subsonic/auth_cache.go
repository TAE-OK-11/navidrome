package subsonic

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"golang.org/x/sync/singleflight"
)

const (
	authUserCacheLimit  = 256
	authUserCacheTTL    = time.Second
	authUserLoadTimeout = 3 * time.Second
)

type authUserCacheEntry struct {
	user    *model.User
	expires time.Time
}

// authUserCache removes the repeated SQLite lookup and password decryption
// caused by bursty Range requests. Credentials are still validated for every
// request; only the immutable user row is reused for at most one second.
type authUserCache struct {
	mu      sync.RWMutex
	entries map[string]authUserCacheEntry
	group   singleflight.Group
	limit   int
	ttl     time.Duration
}

func newAuthUserCache(limit int, ttl time.Duration) *authUserCache {
	return &authUserCache{entries: make(map[string]authUserCacheEntry), limit: limit, ttl: ttl}
}

func (c *authUserCache) get(ctx context.Context, key string, load func(context.Context) (*model.User, error)) (*model.User, error) {
	key = strings.ToLower(key)
	if user, ok := c.lookup(key, time.Now()); ok {
		return user, nil
	}
	result := c.group.DoChan(key, func() (any, error) {
		if user, ok := c.lookup(key, time.Now()); ok {
			return user, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authUserLoadTimeout)
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

func (c *authUserCache) lookup(key string, now time.Time) (*model.User, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	return entry.user, ok && now.Before(entry.expires)
}

func (c *authUserCache) store(key string, user *model.User, now time.Time) {
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
	c.entries[key] = authUserCacheEntry{user: user, expires: now.Add(c.ttl)}
}
