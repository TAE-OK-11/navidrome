package subsonic

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"golang.org/x/sync/singleflight"
)

const (
	// Covers bursty Range/seek requests for the current track without
	// pinning scanner updates for long. The library scanner on this
	// deployment is typically idle, so a few seconds of reuse is safe.
	streamMediaCacheLimit                 = 512
	streamMediaCacheTTL                   = 8 * time.Second
	streamMediaCacheLookupTimeout         = 5 * time.Second
	streamMediaCacheVerifyInterval        = 2 * time.Second
)

type streamMediaCacheEntry struct {
	mediaFile model.MediaFile
	expires   time.Time
	loadedAt  time.Time
}

type streamMediaCache struct {
	mu      sync.RWMutex
	entries map[string]streamMediaCacheEntry
	group   singleflight.Group
	limit   int
	ttl     time.Duration
}

func newStreamMediaCache(limit int, ttl time.Duration) *streamMediaCache {
	return &streamMediaCache{
		entries: make(map[string]streamMediaCacheEntry),
		limit:   limit,
		ttl:     ttl,
	}
}

func streamMediaCacheKey(user model.User, mediaID string) string {
	key := make([]byte, 0, len(user.ID)+len(mediaID)+2+len(user.Libraries)*4)
	key = append(key, user.ID...)
	key = append(key, 0)
	if user.IsAdmin {
		key = append(key, '1')
	} else {
		key = append(key, '0')
		for _, library := range user.Libraries {
			key = strconv.AppendInt(key, int64(library.ID), 10)
			key = append(key, ',')
		}
	}
	key = append(key, 0)
	key = append(key, mediaID...)
	return string(key)
}

func (c *streamMediaCache) get(
	ctx context.Context,
	key string,
	load func(context.Context) (*model.MediaFile, error),
	isStale func(context.Context, model.MediaFile) bool,
) (*model.MediaFile, error) {
	now := time.Now()
	if mediaFile, entry, ok := c.lookupEntry(key, now); ok {
		if isStale == nil || !c.shouldVerify(entry, now) || !isStale(ctx, mediaFile) {
			if isStale != nil && c.shouldVerify(entry, now) {
				c.markVerified(key, now)
			}
			return &mediaFile, nil
		}
		c.delete(key)
	}

	result := c.group.DoChan(key, func() (any, error) {
		if mediaFile, entry, ok := c.lookupEntry(key, time.Now()); ok {
			if isStale == nil || !c.shouldVerify(entry, time.Now()) || !isStale(ctx, mediaFile) {
				return mediaFile, nil
			}
			c.delete(key)
		}

		lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), streamMediaCacheLookupTimeout)
		defer cancel()
		mediaFile, err := load(lookupCtx)
		if err != nil {
			return nil, err
		}
		value := *mediaFile
		c.store(key, value, time.Now())
		return value, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		mediaFile := value.Val.(model.MediaFile)
		return &mediaFile, nil
	}
}

func (c *streamMediaCache) lookupEntry(key string, now time.Time) (model.MediaFile, streamMediaCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || !now.Before(entry.expires) {
		return model.MediaFile{}, streamMediaCacheEntry{}, false
	}
	return entry.mediaFile, entry, true
}

func (c *streamMediaCache) shouldVerify(entry streamMediaCacheEntry, now time.Time) bool {
	return now.Sub(entry.loadedAt) >= streamMediaCacheVerifyInterval
}

func (c *streamMediaCache) markVerified(key string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return
	}
	entry.loadedAt = now
	c.entries[key] = entry
}

func (c *streamMediaCache) delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *streamMediaCache) store(key string, mediaFile model.MediaFile, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.limit {
		for existingKey, entry := range c.entries {
			if !now.Before(entry.expires) {
				delete(c.entries, existingKey)
			}
		}
		if len(c.entries) >= c.limit {
			oldestKey := ""
			var oldestExpiry time.Time
			for existingKey, entry := range c.entries {
				if oldestKey == "" || entry.expires.Before(oldestExpiry) {
					oldestKey = existingKey
					oldestExpiry = entry.expires
				}
			}
			if oldestKey != "" {
				delete(c.entries, oldestKey)
			}
		}
	}
	c.entries[key] = streamMediaCacheEntry{
		mediaFile: mediaFile,
		expires:   now.Add(c.ttl),
		loadedAt:  now,
	}
}

func (c *streamMediaCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]streamMediaCacheEntry)
}

func (api *Router) mediaFileForStreaming(ctx context.Context, id string) (*model.MediaFile, error) {
	user, ok := request.UserFrom(ctx)
	if !ok || api.streamFiles == nil {
		return api.ds.MediaFile(ctx).GetForStreaming(id)
	}
	key := streamMediaCacheKey(user, id)
	return api.streamFiles.get(ctx, key, func(lookupCtx context.Context) (*model.MediaFile, error) {
		return api.ds.MediaFile(lookupCtx).GetForStreaming(id)
	}, func(lookupCtx context.Context, cached model.MediaFile) bool {
		updatedAt, err := api.ds.MediaFile(lookupCtx).GetUpdatedAt(id)
		if err != nil {
			return true
		}
		return !updatedAt.Equal(cached.UpdatedAt)
	})
}
