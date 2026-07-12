package hotcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/stretchr/testify/require"
)

func TestResolverMissPromotionHitIntegrityAndRange(t *testing.T) {
	source, mf, expected := createSource(t, "track", bytes.Repeat([]byte("0123456789abcdef"), 4096))
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 4<<20)

	miss, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	require.False(t, IsHit(miss))
	contents, err := io.ReadAll(miss)
	require.NoError(t, err)
	require.Equal(t, expected, contents)
	ObservePlayback(miss, context.Background(), PlaybackObservation{Playback: true, Method: http.MethodGet, Elapsed: 21 * time.Second})
	require.NoError(t, miss.Close())
	require.Equal(t, uint64(1), r.Stats().Misses)
	waitForIdle(t, r)
	require.Equal(t, uint64(1), r.Stats().Promotions)

	hit, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	require.True(t, IsHit(hit))
	connection, err := hit.SyscallConn()
	require.NoError(t, err)
	require.NotNil(t, connection)
	require.Equal(t, expected, readAndClose(t, hit))
	require.Equal(t, uint64(1), r.Stats().Hits)

	rangeFile, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	defer rangeFile.Close()
	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	request.Header.Set("Range", "bytes=101-4096")
	response := httptest.NewRecorder()
	http.ServeContent(response, request, filepath.Base(source), time.Now(), rangeFile)
	require.Equal(t, http.StatusPartialContent, response.Code)
	require.Equal(t, expected[101:4097], response.Body.Bytes())

	entry := onlyEntry(t, r)
	cached, err := os.ReadFile(entry.dataPath)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(expected), sha256.Sum256(cached))
}

func TestResolverDeduplicatesConcurrentPromotion(t *testing.T) {
	_, mf, expected := createSource(t, "concurrent", bytes.Repeat([]byte("a"), 512<<10))
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 4<<20)
	started := make(chan struct{})
	continueCopy := make(chan struct{})
	var copies atomic.Int32
	r.copyFile = func(_ context.Context, dst io.Writer, src io.Reader, buffer []byte, _ *promotionTask) (int64, error) {
		if copies.Add(1) == 1 {
			close(started)
		}
		<-continueCopy
		return io.CopyBuffer(dst, src, buffer)
	}

	first, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	contents, err := io.ReadAll(first)
	require.NoError(t, err)
	require.Equal(t, expected, contents)
	ObservePlayback(first, context.Background(), PlaybackObservation{Playback: true, Method: http.MethodGet, Elapsed: 21 * time.Second})
	require.NoError(t, first.Close())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("promotion did not start")
	}

	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			file, openErr := r.Open(context.Background(), mf)
			require.NoError(t, openErr)
			_, _ = io.Copy(io.Discard, file)
			require.NoError(t, file.Close())
		})
	}
	group.Wait()
	close(continueCopy)
	waitForIdle(t, r)
	require.Equal(t, int32(1), copies.Load())
	require.Equal(t, uint64(1), r.Stats().Promotions)
	require.Equal(t, 1, r.Stats().Entries)
}

func TestResolverDefersPromotionUntilPlaybackCloses(t *testing.T) {
	_, mf, expected := createSource(t, "deferred", bytes.Repeat([]byte("d"), 4096))
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 1<<20)
	started := make(chan struct{})
	r.copyFile = func(_ context.Context, dst io.Writer, src io.Reader, buffer []byte, _ *promotionTask) (int64, error) {
		close(started)
		return io.CopyBuffer(dst, src, buffer)
	}

	file, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	contents, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, expected, contents)
	ObservePlayback(file, context.Background(), PlaybackObservation{Playback: true, Method: http.MethodGet, Elapsed: 21 * time.Second})
	select {
	case <-started:
		t.Fatal("promotion started while direct playback was active")
	default:
	}
	require.NoError(t, file.Close())
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("promotion did not start after playback closed")
	}
	waitForIdle(t, r)
}

func TestResolverEvictsLeastRecentlyUsedEntry(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache")
	r := newTestResolver(t, cachePath, 3500)
	_, first, _ := createSource(t, "first", bytes.Repeat([]byte("1"), 900))
	_, second, _ := createSource(t, "second", bytes.Repeat([]byte("2"), 900))
	_, third, _ := createSource(t, "third", bytes.Repeat([]byte("3"), 900))

	promoteAndWait(t, r, first)
	time.Sleep(5 * time.Millisecond)
	promoteAndWait(t, r, second)
	time.Sleep(5 * time.Millisecond)
	touched, err := r.Open(context.Background(), first)
	require.NoError(t, err)
	require.NoError(t, touched.Close())
	time.Sleep(5 * time.Millisecond)
	promoteAndWait(t, r, third)

	r.mu.Lock()
	_, hasFirst := r.entries[keyFor(first.ID, first.AbsolutePath())]
	_, hasSecond := r.entries[keyFor(second.ID, second.AbsolutePath())]
	_, hasThird := r.entries[keyFor(third.ID, third.AbsolutePath())]
	r.mu.Unlock()
	require.True(t, hasFirst)
	require.False(t, hasSecond)
	require.True(t, hasThird)
	require.GreaterOrEqual(t, r.Stats().Evictions, uint64(1))
	require.LessOrEqual(t, r.Stats().Bytes, int64(3500))
}

func TestResolverDoesNotEvictActiveEntry(t *testing.T) {
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 2600)
	_, first, _ := createSource(t, "pinned", bytes.Repeat([]byte("p"), 900))
	_, second, expectedSecond := createSource(t, "waiting", bytes.Repeat([]byte("w"), 900))
	promoteAndWait(t, r, first)

	pinned, err := r.Open(context.Background(), first)
	require.NoError(t, err)
	secondMiss, err := r.Open(context.Background(), second)
	require.NoError(t, err)
	require.Equal(t, expectedSecond, readAndClose(t, secondMiss))
	require.NoError(t, r.Promote(context.Background(), second))
	waitForIdle(t, r)
	require.Equal(t, 1, r.Stats().Entries)
	require.GreaterOrEqual(t, r.Stats().Failures, uint64(1))
	require.NoError(t, pinned.Close())

	promoteAndWait(t, r, second)
	require.Equal(t, 1, r.Stats().Entries)
}

func TestResolverInvalidatesChangedAndDeletedSources(t *testing.T) {
	source, mf, original := createSource(t, "mutable", bytes.Repeat([]byte("old"), 1024))
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 1<<20)
	promoteAndWait(t, r, mf)

	changed := bytes.Repeat([]byte("new-data"), 1024)
	require.NoError(t, os.WriteFile(source, changed, 0o600))
	modified, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	require.Equal(t, changed, readAndClose(t, modified))
	require.NotEqual(t, original, changed)
	require.Zero(t, r.Stats().Entries)
	require.Equal(t, uint64(1), r.Status().SourceInvalidations)
	require.Equal(t, uint64(1), r.Status().Fallbacks)
	promoteAndWait(t, r, mf)
	verified, err := r.Open(context.Background(), mf)
	require.NoError(t, err)
	require.Equal(t, changed, readAndClose(t, verified))

	require.NoError(t, os.Remove(source))
	_, err = r.Open(context.Background(), mf)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Equal(t, 0, r.Stats().Entries)
}

func TestResolverRecoversTemporaryAndCorruptFiles(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache")
	_, mf, _ := createSource(t, "recover", bytes.Repeat([]byte("r"), 4096))
	r := newTestResolver(t, cachePath, 1<<20)
	promoteAndWait(t, r, mf)
	entry := onlyEntry(t, r)
	require.NoError(t, os.Chmod(entry.dataPath, 0o600))
	file, err := os.OpenFile(entry.dataPath, os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteAt([]byte("corrupt"), 10)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "interrupted.123.tmp"), []byte("partial"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cachePath, "orphan.data"), []byte("orphan"), 0o600))

	recovered := newTestResolver(t, cachePath, 1<<20)
	require.Equal(t, 0, recovered.Stats().Entries)
	items, err := os.ReadDir(cachePath)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestResolverFallsBackForUnavailableCacheAndDiskFull(t *testing.T) {
	_, mf, expected := createSource(t, "fallback", bytes.Repeat([]byte("f"), 4096))
	invalidPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(invalidPath, []byte("occupied"), 0o600))
	disabled := New(Options{Enabled: true, Path: invalidPath, MaxSize: 1 << 20, PromoteOnPlay: true})
	file, err := disabled.Open(context.Background(), mf)
	require.NoError(t, err)
	require.Equal(t, expected, readAndClose(t, file))

	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 1<<20)
	r.copyFile = func(context.Context, io.Writer, io.Reader, []byte, *promotionTask) (int64, error) {
		return 0, syscall.ENOSPC
	}
	file, err = r.Open(context.Background(), mf)
	require.NoError(t, err)
	require.Equal(t, expected, readAndClose(t, file))
	require.NoError(t, r.Promote(context.Background(), mf))
	waitForIdle(t, r)
	require.Equal(t, uint64(1), r.Stats().Failures)
	require.Equal(t, 0, r.Stats().Entries)
}

func TestResolverCapacityIncludesInFlightReservation(t *testing.T) {
	_, mf, _ := createSource(t, "reservation", bytes.Repeat([]byte("x"), 256<<10))
	r := newTestResolver(t, filepath.Join(t.TempDir(), "cache"), 512<<10)
	started := make(chan struct{})
	continueCopy := make(chan struct{})
	r.copyFile = func(_ context.Context, dst io.Writer, src io.Reader, buffer []byte, _ *promotionTask) (int64, error) {
		close(started)
		<-continueCopy
		return io.CopyBuffer(dst, src, buffer)
	}
	require.NoError(t, r.Promote(context.Background(), mf))
	<-started
	r.mu.Lock()
	require.Greater(t, r.reserved, int64(256<<10))
	require.LessOrEqual(t, r.used+r.reserved, r.maxSize)
	r.mu.Unlock()
	close(continueCopy)
	waitForIdle(t, r)
}

func TestResolverEnforcesThreeGiBHardLimit(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "cache")
	r := newTestResolver(t, cachePath, 8<<30)
	require.Equal(t, int64(3<<30), r.maxSize)
}

func BenchmarkResolverDirectAndHotHit(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark-audio-block"), 1<<15)
	_, mf, _ := createSource(b, "benchmark", data)
	r := newTestResolver(b, filepath.Join(b.TempDir(), "cache"), 8<<20)
	disabled := New(Options{})
	promoteAndWait(b, r, mf)

	b.Run("direct-source", func(b *testing.B) {
		for range b.N {
			file, err := os.Open(mf.AbsolutePath())
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, file)
			_ = file.Close()
		}
	})
	b.Run("resolver-disabled", func(b *testing.B) {
		for range b.N {
			file, err := disabled.Open(context.Background(), mf)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, file)
			_ = file.Close()
		}
	})
	b.Run("hot-cache-hit", func(b *testing.B) {
		for range b.N {
			file, err := r.Open(context.Background(), mf)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, file)
			_ = file.Close()
		}
	})
}

func BenchmarkManagerMediaStates(b *testing.B) {
	r := &resolver{
		entries:   make(map[string]*entry, 3000),
		promoting: make(map[string]*promotionTask),
	}
	requested := make([]string, 0, 25)
	for i := range 3000 {
		mediaID := fmt.Sprintf("benchmark-media-%04d", i)
		key := keyFor(mediaID, "")
		r.entries[key] = &entry{meta: metadata{Key: key, SourceID: mediaID}}
		if i%120 == 0 {
			requested = append(requested, mediaID)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		states := r.MediaStates(requested)
		if len(states) != len(requested) {
			b.Fatalf("got %d states, want %d", len(states), len(requested))
		}
	}
}

func BenchmarkManagerEntries(b *testing.B) {
	r := &resolver{entries: make(map[string]*entry, 3000), playing: make(map[string]int)}
	now := time.Now()
	for i := range 3000 {
		mediaID := fmt.Sprintf("benchmark-media-%04d", i)
		key := keyFor(mediaID, "")
		r.entries[key] = &entry{
			meta:     metadata{Key: key, SourceID: mediaID, Title: mediaID, DataSize: 4 << 20},
			lastUsed: now.Add(-time.Duration(i) * time.Second),
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		page := r.Entries(EntryQuery{Sort: "recent", Order: "desc", Limit: 25})
		if page.Total != 3000 || len(page.Items) != 25 {
			b.Fatalf("got total=%d items=%d", page.Total, len(page.Items))
		}
	}
}

type testingTB interface {
	Helper()
	TempDir() string
	Fatalf(string, ...any)
	Cleanup(func())
}

func newTestResolver(t testingTB, path string, maxSize int64) *resolver {
	t.Helper()
	created := New(Options{
		Enabled: true, Path: path, MaxSize: maxSize, PromoteOnPlay: true,
		SessionWindow: 30 * time.Second, SessionIdleTimeout: 60 * time.Second,
		MaxSessions: 1024, MinPlaySeconds: 20, MinPlayPercent: 25,
		QueueMax: 128, PromotionDelayAfterPlay: 0, PromotionMaxRetries: 0,
		TouchInterval: time.Hour, StatsEnabled: true, EventsMax: 256,
	})
	r, ok := created.(*resolver)
	if !ok || !r.enabled {
		t.Fatalf("hot cache was not enabled")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.Shutdown(ctx)
	})
	return r
}

func createSource(t testingTB, id string, data []byte) (string, *model.MediaFile, []byte) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, id+".flac")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path, &model.MediaFile{ID: id, LibraryPath: directory, Path: filepath.Base(path), Suffix: "flac", Size: int64(len(data))}, data
}

func readAndClose(t *testing.T, file File) []byte {
	t.Helper()
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	return data
}

func promoteAndWait(t testingTB, r *resolver, mf *model.MediaFile) {
	t.Helper()
	if err := r.Promote(context.Background(), mf); err != nil {
		t.Fatalf("promote source: %v", err)
	}
	waitForIdle(t, r)
}

func waitForIdle(t testingTB, r *resolver) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		idle := len(r.promoting) == 0 && r.reserved == 0
		r.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("hot-cache promotion did not become idle: %+v", r.Stats())
}

func onlyEntry(t *testing.T, r *resolver) *entry {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.entries, 1)
	for _, cached := range r.entries {
		return cached
	}
	panic(fmt.Sprintf("unreachable: %#v", r.entries))
}
