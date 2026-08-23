package stream

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"golang.org/x/sync/singleflight"
)

const (
	transcodingProfileCacheTTL    = 5 * time.Second
	transcodingProfileCacheLimit  = 64
	transcodingProfileLoadTimeout = 3 * time.Second
)

type transcodingProfileCacheEntry struct {
	value   model.Transcoding
	found   bool
	expires time.Time
}

// transcodingProfileCache removes repeated SQLite reads from the playback hot
// path while keeping administrator edits visible within a few seconds.
type transcodingProfileCache struct {
	mu      sync.RWMutex
	entries map[string]transcodingProfileCacheEntry
	group   singleflight.Group
}

func newTranscodingProfileCache() *transcodingProfileCache {
	return &transcodingProfileCache{entries: make(map[string]transcodingProfileCacheEntry)}
}

func (c *transcodingProfileCache) get(ctx context.Context, ds model.DataStore, format string) *model.Transcoding {
	if value, found, ok := c.lookup(format, time.Now()); ok {
		if !found {
			return nil
		}
		return &value
	}

	value, _, _ := c.group.Do(format, func() (any, error) {
		if value, found, ok := c.lookup(format, time.Now()); ok {
			if !found {
				return nil, nil
			}
			return value, nil
		}

		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), transcodingProfileLoadTimeout)
		defer cancel()
		profile, err := ds.Transcoding(loadCtx).FindByFormat(format)
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				c.store(format, model.Transcoding{}, false, time.Now())
			}
			return nil, nil
		}
		value := *profile
		c.store(format, value, true, time.Now())
		return value, nil
	})
	if value == nil {
		return nil
	}
	profile := value.(model.Transcoding)
	return &profile
}

func (c *transcodingProfileCache) lookup(format string, now time.Time) (model.Transcoding, bool, bool) {
	c.mu.RLock()
	entry, ok := c.entries[format]
	c.mu.RUnlock()
	return entry.value, entry.found, ok && now.Before(entry.expires)
}

func (c *transcodingProfileCache) store(format string, value model.Transcoding, found bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[format]; !exists && len(c.entries) >= transcodingProfileCacheLimit {
		for key, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, key)
			}
		}
		if len(c.entries) >= transcodingProfileCacheLimit {
			for key := range c.entries {
				delete(c.entries, key)
				break
			}
		}
	}
	c.entries[format] = transcodingProfileCacheEntry{
		value: value, found: found, expires: now.Add(transcodingProfileCacheTTL),
	}
}
