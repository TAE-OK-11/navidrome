package subsonic

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/responses"
)

const (
	entityResponseCacheTTL   = 45 * time.Second
	entityResponseCacheLimit = 256
)

type entityResponseCacheEntry struct {
	expires time.Time
	value   *responses.Subsonic
}

type entityResponseCache struct {
	mu      sync.RWMutex
	entries map[string]entityResponseCacheEntry
}

func (c *entityResponseCache) get(key string, now time.Time) (*responses.Subsonic, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || !now.Before(entry.expires) {
		if ok {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
		return nil, false
	}
	return entry.value, true
}

func (c *entityResponseCache) put(key string, now time.Time, value *responses.Subsonic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]entityResponseCacheEntry)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= entityResponseCacheLimit {
		for candidate, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, candidate)
			}
		}
	}
	if len(c.entries) >= entityResponseCacheLimit {
		var oldestKey string
		var oldestExpiry time.Time
		for candidate, entry := range c.entries {
			if oldestKey == "" || entry.expires.Before(oldestExpiry) {
				oldestKey, oldestExpiry = candidate, entry.expires
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = entityResponseCacheEntry{value: value, expires: now.Add(entityResponseCacheTTL)}
}

func (c *entityResponseCache) delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *entityResponseCache) deleteBySuffix(suffix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if strings.HasSuffix(key, suffix) {
			delete(c.entries, key)
		}
	}
}

func (c *entityResponseCache) deleteByEntityID(id string) {
	c.deleteBySuffix("|" + id)
}

func entityResponseCacheKey(r *http.Request, kind, id string) string {
	user, ok := request.UserFrom(r.Context())
	userKey := ""
	if ok {
		userKey = genreResponseCacheKey(user)
	}
	return userKey + "|" + kind + "|" + id
}

func (api *Router) cachedSubsonicResponse(r *http.Request, cacheKey string, loader func() (*responses.Subsonic, error)) (*responses.Subsonic, error) {
	now := time.Now()
	if cached, ok := api.entityCache.get(cacheKey, now); ok {
		return cached, nil
	}
	resp, err := loader()
	if err != nil {
		return nil, err
	}
	api.entityCache.put(cacheKey, now, resp)
	return resp, nil
}
