package lofty

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuildRequestKeepsFilesInsideLibrary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	e := &extractor{baseDir: root}
	req, err := e.buildRequest([]string{"Artist/Album/01 Song.flac"})
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
	if _, err := e.buildRequest([]string{"../outside.flac"}); err == nil {
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

func TestConvertResponse(t *testing.T) {
	t.Parallel()

	resp := response{
		Protocol: protocolVersion,
		Results: map[string]rawResult{
			"song.m4a": {
				Tags:       map[string][]string{"title": {"Song"}},
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
}

func TestConvertResponseReturnsRequestError(t *testing.T) {
	t.Parallel()

	_, err := convertResponse(response{Errors: map[string]string{"$request": "bad request"}})
	if err == nil || err.Error() != "bad request" {
		t.Fatalf("error = %v, want bad request", err)
	}
}
