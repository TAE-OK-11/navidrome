package subsonic

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/require"
)

func TestAuthUserCacheDeduplicatesBurstAndExpires(t *testing.T) {
	cache := newAuthUserCache(8, 20*time.Millisecond)
	var loads atomic.Int32
	load := func(context.Context) (*model.User, error) {
		loads.Add(1)
		time.Sleep(5 * time.Millisecond)
		return &model.User{ID: "user", UserName: "user", Password: "password"}, nil
	}

	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			user, err := cache.get(context.Background(), "PASSWORD\x00User", load)
			require.NoError(t, err)
			require.Equal(t, "user", user.ID)
		})
	}
	group.Wait()
	require.Equal(t, int32(1), loads.Load())

	time.Sleep(25 * time.Millisecond)
	_, err := cache.get(context.Background(), "password\x00user", load)
	require.NoError(t, err)
	require.Equal(t, int32(2), loads.Load())
}

func BenchmarkAuthUserCacheHit(b *testing.B) {
	cache := newAuthUserCache(8, time.Minute)
	load := func(context.Context) (*model.User, error) {
		return &model.User{ID: "user", UserName: "user", Password: "password"}, nil
	}
	_, _ = cache.get(context.Background(), "password\x00user", load)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = cache.get(context.Background(), "password\x00user", load)
	}
}
