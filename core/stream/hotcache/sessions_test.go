package hotcache

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/stretchr/testify/require"
)

func TestPlaybackPromotionPolicy(t *testing.T) {
	t.Run("ten second skip is not promoted", func(t *testing.T) {
		r, mf := newPolicyResolver(t, 120*time.Second)
		observeSourceRequest(t, r, mf, "", 10*time.Second)
		r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
		waitForIdle(t, r)
		require.Zero(t, r.Stats().Promotions)
		require.Equal(t, uint64(1), r.Status().SkippedSessions)
	})

	t.Run("twenty seconds queues promotion", func(t *testing.T) {
		r, mf := newPolicyResolver(t, 120*time.Second)
		observeSourceRequest(t, r, mf, "", 20*time.Second+time.Millisecond)
		waitForIdle(t, r)
		require.Equal(t, uint64(1), r.Stats().Promotions)
		require.Equal(t, uint64(1), r.Status().ValidSessions)
	})

	t.Run("short track reaches twenty five percent", func(t *testing.T) {
		r, mf := newPolicyResolver(t, 40*time.Second)
		observeSourceRequest(t, r, mf, "", 10*time.Second+time.Millisecond)
		waitForIdle(t, r)
		require.Equal(t, uint64(1), r.Stats().Promotions)
	})
}

func TestFiftyRangeRequestsAreOnePlaySession(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	for index := range 50 {
		start := int64(index * 16 * 1024)
		observeSourceRequest(t, r, mf, fmt.Sprintf("bytes=%d-%d", start, start+16*1024-1), 10*time.Millisecond)
	}

	sessions := r.Sessions()
	require.Len(t, sessions, 1)
	require.Equal(t, uint64(50), sessions[0].RequestCount)
	require.Equal(t, uint64(50), sessions[0].RangeCount)
	require.Equal(t, uint64(0), sessions[0].SeekCount)
	require.Equal(t, uint64(50), r.Status().RangeRequests)
	require.Equal(t, uint64(1), r.Status().PlaySessions)
	require.Equal(t, float64(50), r.Status().AverageRangeRequestsSession)
	require.Zero(t, r.Stats().Promotions)
}

func TestProbeRangeIsNotCountedAsPlaySession(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	observeSourceRequest(t, r, mf, "bytes=0-1", 20*time.Millisecond)
	require.Equal(t, uint64(1), r.Status().RangeRequests)
	require.Zero(t, r.Status().PlaySessions)
	require.Zero(t, r.Stats().Promotions)
	r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
	require.Zero(t, r.Status().SkippedSessions)
}

func TestSeekCannotInflatePlaybackProgress(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	observeSourceRequest(t, r, mf, "bytes=0-65535", time.Second)
	observeSourceRequest(t, r, mf, fmt.Sprintf("bytes=%d-%d", mf.Size-65536, mf.Size-1), time.Second)

	sessions := r.Sessions()
	require.Len(t, sessions, 1)
	require.Equal(t, uint64(1), sessions[0].SeekCount)
	require.Equal(t, uint64(1), r.Status().SeekOperations)
	require.Less(t, sessions[0].PlayedDuration, 20*time.Second)
	require.Zero(t, r.Stats().Promotions)
}

func TestDifferentPlayerAndTimeoutCreateNewSessions(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	ctxOne := playbackContext("user", "player-one")
	ctxTwo := playbackContext("user", "player-two")
	observeSourceRequestWithContext(t, r, mf, ctxOne, "bytes=0-65535", time.Second)
	observeSourceRequestWithContext(t, r, mf, ctxTwo, "bytes=0-65535", time.Second)
	require.Equal(t, uint64(2), r.Status().PlaySessions)

	r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
	observeSourceRequestWithContext(t, r, mf, ctxOne, "bytes=65536-131071", time.Second)
	require.Equal(t, uint64(3), r.Status().PlaySessions)
}

func TestCancelledSessionEndsAfterGracePeriod(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	identity := newStreamIdentity(playbackContext("user", "player"), mf, mf.AbsolutePath(), mf.Size, time.Now().UnixNano())
	observation := PlaybackObservation{
		Playback: true, Method: http.MethodGet, RangeHeader: "bytes=0-65535", Elapsed: time.Second,
		RemoteAddr: "127.0.0.1:1234", UserAgent: "cancelled-client", Cancelled: true,
	}
	r.sessions.observe(context.Background(), identity, false, observation)
	reference := time.Now()

	r.sessions.cleanup(reference.Add(cancelledSessionGrace/2), false)
	require.Len(t, r.Sessions(), 1)
	r.sessions.cleanup(reference.Add(cancelledSessionGrace+time.Second), false)
	require.Empty(t, r.Sessions())
}

func TestReplacementRangeKeepsCancelledSessionActive(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	identity := newStreamIdentity(playbackContext("user", "player"), mf, mf.AbsolutePath(), mf.Size, time.Now().UnixNano())
	observation := PlaybackObservation{
		Playback: true, Method: http.MethodGet, RangeHeader: "bytes=0-65535", Elapsed: time.Second,
		RemoteAddr: "127.0.0.1:1234", UserAgent: "seeking-client", Cancelled: true,
	}
	r.sessions.observe(context.Background(), identity, false, observation)
	r.sessions.begin(identity, observation)

	r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
	require.Len(t, r.Sessions(), 1)

	observation.Cancelled = false
	observation.RangeHeader = "bytes=65536-131071"
	r.sessions.observe(context.Background(), identity, false, observation)
	require.Len(t, r.Sessions(), 1)
}

func TestSessionLimitIsRaceSafeAndCleanupIsBounded(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	r.sessions.maxSessions = 2
	var group sync.WaitGroup
	for index := range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			identity := newStreamIdentity(playbackContext("user", fmt.Sprintf("player-%d", index)), mf,
				mf.AbsolutePath(), mf.Size, time.Now().UnixNano())
			r.sessions.observe(context.Background(), identity, false, PlaybackObservation{
				Playback: true, Method: http.MethodGet, RangeHeader: "bytes=0-1023", Elapsed: time.Millisecond,
				RemoteAddr: fmt.Sprintf("127.0.0.1:%d", index),
			})
		}()
	}
	group.Wait()
	require.LessOrEqual(t, r.sessions.count.Load(), int64(2))
	r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
	require.Zero(t, r.sessions.count.Load())
	require.Empty(t, r.Sessions())
}

func TestRangeStressKeepsSessionsAndEventsBounded(t *testing.T) {
	r, mf := newPolicyResolver(t, 100*time.Second)
	identity := newStreamIdentity(playbackContext("user", "stress-player"), mf, mf.AbsolutePath(), mf.Size, time.Now().UnixNano())
	observation := PlaybackObservation{
		Playback: true, Method: http.MethodGet, RangeHeader: "bytes=0-1023", Elapsed: time.Millisecond,
		RemoteAddr: "127.0.0.1:1234", UserAgent: "stress-client", Sendfile: true,
	}
	for range 100_000 {
		r.sessions.observe(context.Background(), identity, true, observation)
	}
	require.Equal(t, int64(1), r.sessions.count.Load())
	require.LessOrEqual(t, len(r.Events(0, 1000)), 128)
	require.Equal(t, uint64(100_000), r.Status().TotalHTTPRequests)
	r.sessions.cleanup(time.Now().Add(2*time.Minute), false)
	require.Zero(t, r.sessions.count.Load())
}

func TestRangeNormalizationAndUnion(t *testing.T) {
	tests := []struct {
		header     string
		start, end int64
		valid      bool
	}{
		{"bytes=10-19", 10, 19, true},
		{"bytes=10-", 10, 99, true},
		{"bytes=-10", 90, 99, true},
		{"bytes=100-101", 0, 0, false},
		{"bytes=0-1,4-5", 0, 0, false},
	}
	for _, test := range tests {
		actual := normalizeRange(test.header, 100)
		require.Equal(t, test.valid, actual.valid, test.header)
		if test.valid {
			require.Equal(t, test.start, actual.start, test.header)
			require.Equal(t, test.end, actual.end, test.header)
		}
	}

	var intervals []byteInterval
	require.Equal(t, int64(10), addInterval(&intervals, byteInterval{0, 9}))
	require.Equal(t, int64(5), addInterval(&intervals, byteInterval{5, 14}))
	require.Equal(t, int64(5), addInterval(&intervals, byteInterval{20, 24}))
	require.Equal(t, int64(5), addInterval(&intervals, byteInterval{15, 19}))
	require.Equal(t, []byteInterval{{0, 24}}, intervals)
}

func newPolicyResolver(t testingTB, duration time.Duration) (*resolver, *model.MediaFile) {
	t.Helper()
	data := bytes.Repeat([]byte("audio-block"), 100_000)
	_, mf, _ := createSource(t, "policy", data)
	mf.Duration = float32(duration.Seconds())
	mf.Size = int64(len(data))
	created := New(Options{
		Enabled: true, Path: filepath.Join(t.TempDir(), "cache"), MaxSize: 8 << 20, PromoteOnPlay: true,
		SessionWindow: 30 * time.Second, SessionIdleTimeout: 60 * time.Second, MaxSessions: 64,
		MinPlaySeconds: 20, MinPlayPercent: 25, QueueMax: 8, PromotionDelayAfterPlay: 0,
		PromotionMaxRetries: 0, TouchInterval: time.Hour, StatsEnabled: true, EventsMax: 128,
	})
	r := created.(*resolver)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	return r, mf
}

func observeSourceRequest(t *testing.T, r *resolver, mf *model.MediaFile, rangeHeader string, elapsed time.Duration) {
	t.Helper()
	observeSourceRequestWithContext(t, r, mf, playbackContext("user", "player"), rangeHeader, elapsed)
}

func observeSourceRequestWithContext(t *testing.T, r *resolver, mf *model.MediaFile, ctx context.Context, rangeHeader string, elapsed time.Duration) {
	t.Helper()
	file, err := r.Open(ctx, mf)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, file)
	require.NoError(t, err)
	ObservePlayback(file, ctx, PlaybackObservation{
		Playback: true, Method: http.MethodGet, RangeHeader: rangeHeader, Elapsed: elapsed,
		RemoteAddr: "127.0.0.1:1234", UserAgent: "hot-cache-test", Sendfile: true,
	})
	require.NoError(t, file.Close())
}

func playbackContext(userID, playerID string) context.Context {
	ctx := request.WithUser(context.Background(), model.User{ID: userID, UserName: userID})
	return request.WithPlayer(ctx, model.Player{ID: playerID, Name: playerID, Client: playerID})
}

func BenchmarkSessionRangeObservation(b *testing.B) {
	r, mf := newPolicyResolver(b, 180*time.Second)
	identity := newStreamIdentity(playbackContext("user", "player"), mf, mf.AbsolutePath(), mf.Size, time.Now().UnixNano())
	observation := PlaybackObservation{
		Playback: true, Method: http.MethodGet, RangeHeader: "bytes=0-1048575", Elapsed: 10 * time.Millisecond,
		RemoteAddr: "127.0.0.1:1234", UserAgent: "benchmark", Sendfile: true,
	}
	r.sessions.observe(context.Background(), identity, true, observation)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.sessions.observe(context.Background(), identity, true, observation)
	}
}
