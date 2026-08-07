package subsonic

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/core/stream"
)

func TestHandleArchiveErrBeforeWritePropagates(t *testing.T) {
	want := errors.New("boom")
	if got := handleArchiveErr(context.Background(), "x", false, want); got != want {
		t.Fatalf("error before body write was not preserved: got %v, want %v", got, want)
	}
}

func TestHandleArchiveErrAfterWriteDoesNotAppendOuterError(t *testing.T) {
	if got := handleArchiveErr(context.Background(), "x", true, errors.New("boom")); got != nil {
		t.Fatalf("error after archive body started should not reach the outer response handler: %v", got)
	}
}

func TestHandleArchiveErrBeforeWritePreservesTranscodeLimiter(t *testing.T) {
	got := handleArchiveErr(context.Background(), "x", false, stream.ErrTooManyTranscodes)
	if !errors.Is(got, stream.ErrTooManyTranscodes) {
		t.Fatalf("transcode limiter error was not preserved: %v", got)
	}
}

func TestArchiveResponseWriterTracksWrittenBytes(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &archiveResponseWriter{ResponseWriter: recorder}
	if writer.wrote {
		t.Fatal("writer unexpectedly marked as written before first write")
	}
	if _, err := writer.Write([]byte("zip")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !writer.wrote {
		t.Fatal("writer did not record successful response bytes")
	}
}
