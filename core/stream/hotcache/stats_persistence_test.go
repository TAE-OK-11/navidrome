package hotcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeStatisticsPersistAcrossRestart(t *testing.T) {
	options := testOptions(t)
	first := New(options).(*resolver)
	first.runtime.requestHits.Store(42)
	first.runtime.playSessions.Store(3)
	first.runtime.format[1].requestHits.Store(7)
	require.NoError(t, first.persistRuntimeStats())
	require.NoError(t, first.Shutdown(context.Background()))

	second := New(options).(*resolver)
	t.Cleanup(func() { require.NoError(t, second.Shutdown(context.Background())) })
	require.Equal(t, uint64(42), second.Status().RequestHits)
	require.Equal(t, uint64(3), second.Status().PlaySessions)
	require.Equal(t, uint64(7), second.Formats()[1].RequestHits)
}

func TestInvalidRuntimeStatisticsDoNotDisableCache(t *testing.T) {
	options := testOptions(t)
	statsPath := filepath.Join(filepath.Dir(options.Path), ".hot-cache-stats-v1.json")
	require.NoError(t, os.WriteFile(statsPath, []byte("not-json"), 0o600))

	resolver := New(options).(*resolver)
	t.Cleanup(func() { require.NoError(t, resolver.Shutdown(context.Background())) })
	require.True(t, resolver.enabled)
	require.Zero(t, resolver.Status().RequestHits)
}

func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Enabled: true, Path: filepath.Join(t.TempDir(), "hot-music"), MaxSize: 4 << 20,
		PromoteOnPlay: true, SessionWindow: 30 * time.Second, SessionIdleTimeout: time.Minute,
		MaxSessions: 32, MinPlaySeconds: 20, MinPlayPercent: 25, PromotionConcurrency: 1,
		QueueMax: 8, PromotionDelayAfterPlay: time.Millisecond, PromotionMaxRetries: 0,
		TouchInterval: time.Hour, StatsEnabled: true, EventsMax: 32, StatsFlushInterval: time.Hour,
	}
}
