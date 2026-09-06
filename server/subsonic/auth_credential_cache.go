package subsonic

import (
	"strings"
	"sync"
	"time"
)

const (
	authCredentialCacheLimit = 512
	authCredentialCacheTTL   = 5 * time.Second
)

// authCredentialCache remembers successful Subsonic credential checks briefly
// so bursty /rest/ping polls during playback skip repeated MD5 work.
type authCredentialCache struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	limit   int
	ttl     time.Duration
}

func newAuthCredentialCache(limit int, ttl time.Duration) *authCredentialCache {
	return &authCredentialCache{entries: make(map[string]time.Time), limit: limit, ttl: ttl}
}

func authCredentialCacheKey(username, pass, token, salt, jwt string) string {
	var b strings.Builder
	b.Grow(len(username) + len(pass) + len(token) + len(salt) + len(jwt) + 8)
	b.WriteString(strings.ToLower(username))
	b.WriteByte(0)
	switch {
	case jwt != "":
		b.WriteString("jwt")
		b.WriteByte(0)
		b.WriteString(jwt)
	case token != "":
		b.WriteString("t")
		b.WriteByte(0)
		b.WriteString(token)
		b.WriteByte(0)
		b.WriteString(salt)
	default:
		b.WriteString("p")
		b.WriteByte(0)
		b.WriteString(pass)
	}
	return b.String()
}

func (c *authCredentialCache) seen(key string, now time.Time) bool {
	c.mu.RLock()
	expires, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || !now.Before(expires) {
		if ok {
			c.mu.Lock()
			if current, exists := c.entries[key]; exists && !now.Before(current) {
				delete(c.entries, key)
			}
			c.mu.Unlock()
		}
		return false
	}
	return true
}

func (c *authCredentialCache) remember(key string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.limit {
		for existingKey, expires := range c.entries {
			if !now.Before(expires) {
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
	c.entries[key] = now.Add(c.ttl)
}
