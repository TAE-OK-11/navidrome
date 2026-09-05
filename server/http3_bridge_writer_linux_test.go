//go:build linux

package server

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/navidrome/navidrome/utils/ioutils"
)

type recordingWriter struct {
	httptest.ResponseRecorder
	writes []int
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	if n > 0 {
		w.writes = append(w.writes, n)
	}
	return n, err
}

func TestBridgeFrameWriterBatchesMediaWrites(t *testing.T) {
	rec := &recordingWriter{}
	w := newBridgeFrameWriter(rec, "/rest/stream.view")
	if w == nil {
		t.Fatal("expected media path to use bridge frame writer")
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

func TestBridgeFrameWriterReadFromDelegates(t *testing.T) {
	rec := &recordingWriter{}
	w := newBridgeFrameWriter(rec, "/rest/stream.view")
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

func TestBridgeFrameWriterSkipsNonMedia(t *testing.T) {
	rec := httptest.NewRecorder()
	if newBridgeFrameWriter(rec, "/rest/ping.view") != nil {
		t.Fatal("non-media path should not wrap response writer")
	}
}

var _ io.ReaderFrom = (*bridgeFrameWriter)(nil)
