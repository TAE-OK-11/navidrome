package subsonic

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StreamMediaCache", func() {
	It("reuses recent metadata and returns independent values", func() {
		cache := newStreamMediaCache(4, time.Second)
		var calls atomic.Int32
		load := func(context.Context) (*model.MediaFile, error) {
			calls.Add(1)
			return &model.MediaFile{ID: "song", Title: "original"}, nil
		}

		first, err := cache.get(context.Background(), "user-song", load)
		Expect(err).NotTo(HaveOccurred())
		first.Title = "changed"
		second, err := cache.get(context.Background(), "user-song", load)
		Expect(err).NotTo(HaveOccurred())

		Expect(second.Title).To(Equal("original"))
		Expect(calls.Load()).To(Equal(int32(1)))
	})

	It("reloads expired metadata", func() {
		cache := newStreamMediaCache(4, time.Millisecond)
		var calls atomic.Int32
		load := func(context.Context) (*model.MediaFile, error) {
			return &model.MediaFile{ID: "song", TrackNumber: int(calls.Add(1))}, nil
		}

		first, err := cache.get(context.Background(), "user-song", load)
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(2 * time.Millisecond)
		second, err := cache.get(context.Background(), "user-song", load)
		Expect(err).NotTo(HaveOccurred())

		Expect(first.TrackNumber).To(Equal(1))
		Expect(second.TrackNumber).To(Equal(2))
	})

	It("separates entries by the user's current library scope", func() {
		user := model.User{ID: "user", Libraries: model.Libraries{{ID: 1}}}
		first := streamMediaCacheKey(user, "song")
		user.Libraries = model.Libraries{{ID: 2}}

		Expect(streamMediaCacheKey(user, "song")).NotTo(Equal(first))
	})

	It("lets a canceled waiter leave without canceling the shared load", func() {
		cache := newStreamMediaCache(4, time.Second)
		started := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		load := func(context.Context) (*model.MediaFile, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return &model.MediaFile{ID: "song"}, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		first := make(chan error, 1)
		go func() {
			_, err := cache.get(ctx, "user-song", load)
			first <- err
		}()
		<-started
		second := make(chan error, 1)
		go func() {
			_, err := cache.get(context.Background(), "user-song", load)
			second <- err
		}()

		cancel()
		Expect(<-first).To(MatchError(context.Canceled))
		close(release)
		Expect(<-second).NotTo(HaveOccurred())
		Expect(calls.Load()).To(Equal(int32(1)))
	})

	It("keeps the cache bounded", func() {
		cache := newStreamMediaCache(2, time.Second)
		for _, key := range []string{"one", "two", "three"} {
			_, err := cache.get(context.Background(), key, func(context.Context) (*model.MediaFile, error) {
				return &model.MediaFile{ID: key}, nil
			})
			Expect(err).NotTo(HaveOccurred())
		}

		cache.mu.RLock()
		defer cache.mu.RUnlock()
		Expect(cache.entries).To(HaveLen(2))
	})

	It("does not cache loader errors", func() {
		cache := newStreamMediaCache(2, time.Second)
		var calls atomic.Int32
		load := func(context.Context) (*model.MediaFile, error) {
			if calls.Add(1) == 1 {
				return nil, errors.New("temporary")
			}
			return &model.MediaFile{ID: "song"}, nil
		}

		_, err := cache.get(context.Background(), "user-song", load)
		Expect(err).To(MatchError("temporary"))
		_, err = cache.get(context.Background(), "user-song", load)
		Expect(err).NotTo(HaveOccurred())
		Expect(calls.Load()).To(Equal(int32(2)))
	})
})
