package subsonic

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

func BenchmarkStreamMediaCacheHit(b *testing.B) {
	cache := newStreamMediaCache(streamMediaCacheLimit, streamMediaCacheTTL)
	user := model.User{ID: "bench-user", UserName: "bench", IsAdmin: true}
	key := streamMediaCacheKey(user, "song-1")
	mediaFile := model.MediaFile{ID: "song-1", Title: "Track", Artist: "Artist"}
	cache.store(key, mediaFile, time.Now())

	load := func(context.Context) (*model.MediaFile, error) {
		b.Fatal("stream media cache load should not run on hit")
		return nil, nil
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := cache.get(context.Background(), key, load, nil); err != nil {
			b.Fatal(err)
		}
	}
}
