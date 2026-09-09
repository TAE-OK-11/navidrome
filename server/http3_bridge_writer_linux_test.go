//go:build linux

package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/utils/ioutils"
)

type recordingWriter struct {
	httptest.ResponseRecorder
	writes  []int
	flushed int
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	if n > 0 {
		w.writes = append(w.writes, n)
	}
	return n, err
}

func (w *recordingWriter) Flush() {
	w.flushed++
}

func TestBridgeFrameWriterBatchesDownloadWrites(t *testing.T) {
	rec := &recordingWriter{}
	w := newBridgeFrameWriter(rec, "/rest/download.view")
	if w == nil {
		t.Fatal("expected download path to use bridge frame writer")
	}

	chunk := bytes.Repeat([]byte("a"), ioutils.DefaultCopyBufferSize/2)
	if _, err := w.Write(chunk); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(chunk); err != nil {
		t.Fatal(err)
	}
	if err := w.flush(); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 1 || rec.writes[0] != ioutils.DefaultCopyBufferSize {
		t.Fatalf("writes=%v, want one %d-byte frame", rec.writes, ioutils.DefaultCopyBufferSize)
	}
}

func TestBridgeFrameWriterFlushPushesPartialBuffer(t *testing.T) {
	rec := &recordingWriter{}
	w := newBridgeFrameWriter(rec, "/rest/download")
	chunk := bytes.Repeat([]byte("c"), 1024)
	if _, err := w.Write(chunk); err != nil {
		t.Fatal(err)
	}
	if len(rec.writes) != 0 {
		t.Fatalf("writes=%v, want buffered until Flush", rec.writes)
	}
	w.Flush()
	if len(rec.writes) != 1 || rec.writes[0] != len(chunk) {
		t.Fatalf("writes=%v, want one %d-byte flush", rec.writes, len(chunk))
	}
	if rec.flushed != 1 {
		t.Fatalf("underlying Flush calls=%d, want 1", rec.flushed)
	}
}

func TestBridgeFrameWriterReadFromDelegates(t *testing.T) {
	rec := &recordingWriter{}
	w := newBridgeFrameWriter(rec, "/rest/download.view")
	payload := bytes.Repeat([]byte("b"), ioutils.DefaultCopyBufferSize+1)
	n, err := w.ReadFrom(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("read %d bytes, want %d", n, len(payload))
	}
	if err := w.flush(); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, size := range rec.writes {
		total += size
	}
	if total != len(payload) {
		t.Fatalf("wrote %d bytes, want %d (%v)", total, len(payload), rec.writes)
	}
}

func TestBridgeFrameWriterSkipsLiveStreamAndPing(t *testing.T) {
	rec := httptest.NewRecorder()
	for _, path := range []string{
		"/rest/ping.view",
		"/rest/stream.view",
		"/rest/getTranscodeStream",
		"/rest/getCoverArt.view",
	} {
		if newBridgeFrameWriter(rec, path) != nil {
			t.Fatalf("%s should not wrap response writer", path)
		}
	}
}

func TestIsBridgeCoalescePath(t *testing.T) {
	if !isBridgeCoalescePath("/rest/download.view") {
		t.Fatal("download should coalesce")
	}
	if !isBridgeCoalescePath("/share/d/abc") {
		t.Fatal("share download should coalesce")
	}
	if isBridgeCoalescePath("/rest/stream") {
		t.Fatal("live stream must not coalesce on the Go bridge")
	}
}

var (
	_ io.ReaderFrom = (*bridgeFrameWriter)(nil)
	_ http.Flusher  = (*bridgeFrameWriter)(nil)
)

type failingBridgeWriter struct {
	*httptest.ResponseRecorder
	err    error
	writes int
}

func (w *failingBridgeWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func TestBridgeFrameWriterRetainsFlushFailures(t *testing.T) {
	transportErr := errors.New("transport closed")
	for _, writeErr := range []error{transportErr, nil} {
		want := writeErr
		if want == nil {
			want = io.ErrShortWrite
		}
		rec := &failingBridgeWriter{ResponseRecorder: httptest.NewRecorder(), err: writeErr}
		w := newBridgeFrameWriter(rec, "/rest/download")
		if _, err := w.Write([]byte("last audio chunk")); err != nil {
			t.Fatal(err)
		}
		w.Flush()
		if err := w.flush(); !errors.Is(err, want) {
			t.Fatalf("final flush lost failure: %v, want %v", err, want)
		}
		if n, err := w.Write([]byte("more")); n != 0 || !errors.Is(err, want) {
			t.Fatalf("failed writer accepted more bytes: %d, %v", n, err)
		}
		if rec.writes != 1 {
			t.Fatalf("retried failed response: writes=%d", rec.writes)
		}
	}
}
