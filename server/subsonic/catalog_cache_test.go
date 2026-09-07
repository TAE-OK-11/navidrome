package subsonic

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogCacheCapacityAndExpiry(t *testing.T) {
	var cache catalogCache[int]
	now := time.Now()
	cache.store("", now, 1, 2, time.Second)
	cache.store("second", now.Add(time.Millisecond), 2, 2, time.Second)
	cache.store("second", now.Add(2*time.Millisecond), 3, 2, time.Second)
	if len(cache.entries) != 2 {
		t.Fatal("replacement evicted an unrelated entry")
	}
	cache.store("third", now.Add(3*time.Millisecond), 4, 2, time.Second)
	if _, hit := cache.get("", now); hit {
		t.Fatal("oldest entry was not evicted")
	}
	if _, hit := cache.get("third", now.Add(2*time.Second)); hit {
		t.Fatal("expired entry returned")
	}
}

func TestCatalogCacheCanceledWaiterAndSharedLoad(t *testing.T) {
	var cache catalogCache[int]
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	loader := func(ctx context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return 42, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := cache.load(ctx, "key", 2, time.Minute, loader); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter blocked")
	}
	// Inspect the flight directly to deterministically join before releasing it.
	joined := cache.group.DoChan("0:key", func() (any, error) { t.Error("shared flight disappeared"); return 0, nil })
	release <- struct{}{}
	result := <-joined
	if result.Err != nil || result.Val.(int) != 42 || calls.Load() != 1 {
		t.Fatalf("unexpected result: %+v, calls=%d", result, calls.Load())
	}
}

func TestCatalogCacheInvalidationDoesNotResurrectInflightResult(t *testing.T) {
	var cache catalogCache[int]
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		_, err := cache.load(context.Background(), "key", 2, time.Minute, func(context.Context) (int, error) {
			close(started)
			<-release
			return 1, nil
		})
		done <- err
	}()
	<-started
	cache.invalidate(nil)
	value, err := cache.load(context.Background(), "key", 2, time.Minute, func(context.Context) (int, error) { return 2, nil })
	if err != nil || value != 2 {
		t.Fatalf("post-invalidation request joined stale load: %d, %v", value, err)
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if value, _ := cache.get("key", time.Now()); value != 2 {
		t.Fatalf("stale load replaced fresh snapshot: %d", value)
	}
}

func TestCatalogCacheRetriesErrors(t *testing.T) {
	var cache catalogCache[int]
	for range 2 {
		_, err := cache.load(context.Background(), "key", 2, time.Minute, func(context.Context) (int, error) { return 0, errors.New("unavailable") })
		if err == nil {
			t.Fatal("failure cached as success")
		}
	}
	if _, hit := cache.get("key", time.Now()); hit {
		t.Fatal("cached failed lookup")
	}
}

func BenchmarkCatalogCacheHit(b *testing.B) {
	var cache catalogCache[int]
	cache.store("key", time.Now(), 42, 128, time.Minute)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.get("key", time.Now())
		}
	})
}
