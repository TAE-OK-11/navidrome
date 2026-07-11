package hotcache

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromotionQueueBoundAndCancellation(t *testing.T) {
	r := newTestResolverWithQueue(t, 1)
	_, first, _ := createSource(t, "queue-first", bytes.Repeat([]byte("1"), 64<<10))
	_, second, _ := createSource(t, "queue-second", bytes.Repeat([]byte("2"), 64<<10))
	r.Pause()
	require.NoError(t, r.Promote(context.Background(), first))
	require.ErrorContains(t, r.Promote(context.Background(), second), "queue is full")
	require.Len(t, r.Queue(), 1)
	require.NoError(t, r.Cancel(first.ID))
	r.Resume()
	waitForIdle(t, r)
	require.Zero(t, r.Stats().Promotions)
	require.Zero(t, r.Stats().Entries)
}

func TestPromotionPauseResumeAndSourceRevalidation(t *testing.T) {
	t.Run("pause prevents active copy until resume", func(t *testing.T) {
		r := newTestResolverWithQueue(t, 8)
		_, mf, _ := createSource(t, "pause", bytes.Repeat([]byte("p"), 64<<10))
		started := make(chan struct{})
		r.copyFile = func(_ context.Context, dst io.Writer, src io.Reader, buffer []byte, _ *promotionTask) (int64, error) {
			close(started)
			return io.CopyBuffer(dst, src, buffer)
		}
		r.Pause()
		require.NoError(t, r.Promote(context.Background(), mf))
		select {
		case <-started:
			t.Fatal("copy started while promotions were paused")
		case <-time.After(30 * time.Millisecond):
		}
		r.Resume()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("copy did not resume")
		}
		waitForIdle(t, r)
		require.Equal(t, uint64(1), r.Stats().Promotions)
	})

	t.Run("changed source fails without publishing cache data", func(t *testing.T) {
		r := newTestResolverWithQueue(t, 8)
		path, mf, _ := createSource(t, "changed-queued", bytes.Repeat([]byte("a"), 64<<10))
		r.Pause()
		require.NoError(t, r.Promote(context.Background(), mf))
		require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("b"), 96<<10), 0o600))
		r.Resume()
		waitForIdle(t, r)
		require.Equal(t, uint64(1), r.Stats().Failures)
		require.Zero(t, r.Stats().Entries)
	})
}

func TestLRUTouchIsBatchedOffRequestPath(t *testing.T) {
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 1<<20)
	_, mf, _ := createSource(t, "touch", bytes.Repeat([]byte("t"), 32<<10))
	promoteAndWait(t, r, mf)
	cached := onlyEntry(t, r)
	before, err := os.Stat(cached.metadataPath)
	require.NoError(t, err)

	for range 20 {
		file, openErr := r.Open(context.Background(), mf)
		require.NoError(t, openErr)
		require.NoError(t, file.Close())
	}
	after, err := os.Stat(cached.metadataPath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
	require.Equal(t, uint64(20), cached.requestHits)
}

func TestReadonlyPromotionFailureNeverAffectsSource(t *testing.T) {
	r := newTestResolverWithQueue(t, 8)
	_, mf, expected := createSource(t, "readonly", bytes.Repeat([]byte("r"), 32<<10))
	r.copyFile = func(context.Context, io.Writer, io.Reader, []byte, *promotionTask) (int64, error) {
		return 0, syscall.EROFS
	}
	require.NoError(t, r.Promote(context.Background(), mf))
	waitForIdle(t, r)
	require.Equal(t, uint64(1), r.Stats().Failures)
	file, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	actual, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, expected, actual)
	require.False(t, IsHit(file))
}

func newTestResolverWithQueue(t *testing.T, queueMax int) *resolver {
	t.Helper()
	created := New(Options{
		Enabled: true, Path: filepath.Join(t.TempDir(), "cache"), MaxSize: 4 << 20, PromoteOnPlay: true,
		SessionWindow: 30 * time.Second, SessionIdleTimeout: 60 * time.Second, MaxSessions: 64,
		MinPlaySeconds: 20, MinPlayPercent: 25, QueueMax: queueMax, PromotionDelayAfterPlay: 0,
		PromotionMaxRetries: 0, TouchInterval: time.Hour, StatsEnabled: true, EventsMax: 128,
	})
	r := created.(*resolver)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	return r
}
