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
	// Stream metadata is deliberately short-lived: it covers bursty Range
	// requests without hiding scanner updates for more than one second.
	streamMediaCacheLimit         = 256
	streamMediaCacheTTL           = time.Second
	streamMediaCacheLookupTimeout = 5 * time.Second
)

type streamMediaCacheEntry struct {
	mediaFile model.MediaFile
	expires   time.Time
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

func (c *streamMediaCache) get(ctx context.Context, key string, load func(context.Context) (*model.MediaFile, error)) (*model.MediaFile, error) {
	if mediaFile, ok := c.lookup(key, time.Now()); ok {
		return &mediaFile, nil
	}

	result := c.group.DoChan(key, func() (any, error) {
		if mediaFile, ok := c.lookup(key, time.Now()); ok {
			return mediaFile, nil
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

func (c *streamMediaCache) lookup(key string, now time.Time) (model.MediaFile, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	return entry.mediaFile, ok && now.Before(entry.expires)
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
			for existingKey := range c.entries {
				delete(c.entries, existingKey)
				break
			}
		}
	}
	c.entries[key] = streamMediaCacheEntry{mediaFile: mediaFile, expires: now.Add(c.ttl)}
}

func (api *Router) mediaFileForStreaming(ctx context.Context, id string) (*model.MediaFile, error) {
	user, ok := request.UserFrom(ctx)
	if !ok || api.streamFiles == nil {
		return api.ds.MediaFile(ctx).GetForStreaming(id)
	}
	key := streamMediaCacheKey(user, id)
	return api.streamFiles.get(ctx, key, func(lookupCtx context.Context) (*model.MediaFile, error) {
		return api.ds.MediaFile(lookupCtx).GetForStreaming(id)
	})
}
