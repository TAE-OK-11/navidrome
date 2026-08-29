package lofty

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTripHonorsCancellationWhileWaitingForWorker(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := make(chan *workerSlot)
	_, err := (&extractor{}).roundTrip(ctx, pool, request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("roundTrip() error = %v, want context.Canceled", err)
	}
}

func TestBuildRequestKeepsFilesInsideLibrary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	e := &extractor{baseDir: root}
	req, err := e.buildRequest(context.Background(), []string{"Artist/Album/01 Song.flac"})
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if len(req.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(req.Files))
	}
	want := filepath.Join(root, "Artist", "Album", "01 Song.flac")
	if req.Files[0].Path != want {
		t.Fatalf("path = %q, want %q", req.Files[0].Path, want)
	}
}

func TestBuildRequestRejectsTraversal(t *testing.T) {
	t.Parallel()

	e := &extractor{baseDir: t.TempDir()}
	if _, err := e.buildRequest(context.Background(), []string{"../outside.flac"}); err == nil {
		t.Fatal("buildRequest() accepted path traversal")
	}
}

func TestWorkerPoolSize(t *testing.T) {
	t.Parallel()

	if got := workerPoolSize(0); got != 1 {
		t.Fatalf("workerPoolSize(0) = %d, want 1", got)
	}
	if got := workerPoolSize(5); got != 5 {
		t.Fatalf("workerPoolSize(5) = %d, want 5", got)
	}
	if got := workerPoolSize(100); got != maxWorkerPool {
		t.Fatalf("workerPoolSize(100) = %d, want %d", got, maxWorkerPool)
	}
}

func TestMetadataTaskCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		files     int
		poolSize  int
		wantTasks int
	}{
		{name: "empty", files: 0, poolSize: 8, wantTasks: 1},
		{name: "single worker", files: 200, poolSize: 1, wantTasks: 1},
		{name: "small batch", files: minFilesPerWorkerTask, poolSize: 8, wantTasks: 1},
		{name: "large folder", files: 200, poolSize: 8, wantTasks: 7},
		{name: "bounded by pool", files: 4096, poolSize: 8, wantTasks: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := metadataTaskCount(tt.files, tt.poolSize); got != tt.wantTasks {
				t.Fatalf("metadataTaskCount(%d, %d) = %d, want %d", tt.files, tt.poolSize, got, tt.wantTasks)
			}
		})
	}
}

func TestConvertResponse(t *testing.T) {
	t.Parallel()

	modified := time.Date(2026, time.August, 26, 6, 30, 0, 123, time.UTC)
	created := modified.Add(-time.Hour)
	createdNS := created.UnixNano()
	resp := response{
		Protocol: protocolVersion,
		Results: map[string]rawResult{
			"song.m4a": {
				Tags:       map[string][]string{"title": {"Song"}},
				FileInfo:   &rawFileInfo{Name: "song.m4a", Size: 1234, ModifiedNS: modified.UnixNano(), CreatedNS: &createdNS},
				DurationNS: uint64((3*time.Minute + 12*time.Second).Nanoseconds()),
				BitRate:    256,
				BitDepth:   16,
				SampleRate: 44100,
				Channels:   2,
				Codec:      "alac",
				HasPicture: true,
			},
		},
	}
	got, err := convertResponse(resp)
	if err != nil {
		t.Fatalf("convertResponse() error = %v", err)
	}
	info := got["song.m4a"]
	if info.AudioProperties.Codec != "alac" {
		t.Fatalf("codec = %q, want alac", info.AudioProperties.Codec)
	}
	if info.AudioProperties.Duration != 3*time.Minute+12*time.Second {
		t.Fatalf("duration = %s", info.AudioProperties.Duration)
	}
	if !info.HasPicture {
		t.Fatal("HasPicture = false, want true")
	}
	if info.FileInfo == nil {
		t.Fatal("FileInfo = nil")
	}
	if info.FileInfo.Name() != "song.m4a" || info.FileInfo.Size() != 1234 {
		t.Fatalf("FileInfo = %q, %d", info.FileInfo.Name(), info.FileInfo.Size())
	}
	if !info.FileInfo.ModTime().Equal(modified) || !info.FileInfo.BirthTime().Equal(created) {
		t.Fatalf("file times = %s, %s", info.FileInfo.ModTime(), info.FileInfo.BirthTime())
	}
}

func TestConvertResponseAllowsOlderWorkerWithoutFileInfo(t *testing.T) {
	t.Parallel()

	got, err := convertResponse(response{Results: map[string]rawResult{"song.mp3": {}}})
	if err != nil {
		t.Fatalf("convertResponse() error = %v", err)
	}
	if got["song.mp3"].FileInfo != nil {
		t.Fatal("older worker result unexpectedly populated FileInfo")
	}
}

func TestConvertResponseReturnsRequestError(t *testing.T) {
	t.Parallel()

	_, err := convertResponse(response{Errors: map[string]string{"$request": "bad request"}})
	if err == nil || err.Error() != "bad request" {
		t.Fatalf("error = %v, want bad request", err)
	}
}
