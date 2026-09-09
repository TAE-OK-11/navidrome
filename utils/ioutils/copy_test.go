package ioutils

import (
	"bytes"
	"io"
	"os"
	"testing"
)

type readerFromWriter struct {
	io.Writer
	called bool
}

func (w *readerFromWriter) ReadFrom(r io.Reader) (int64, error) {
	w.called = true
	return io.Copy(w.Writer, r)
}

type testPipeReader struct {
	*os.File
}

func (t testPipeReader) UnderlyingFile() *os.File { return t.File }

func (t testPipeReader) ExitError() error { return nil }

func TestCopyUsesReaderFromWhenAvailable(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	dst := &readerFromWriter{Writer: io.Discard}
	if _, err := Copy(dst, testPipeReader{File: r}); err != nil {
		t.Fatal(err)
	}
	if !dst.called {
		t.Fatal("expected Copy to delegate to io.ReaderFrom")
	}
}

func TestCopyFallsBackToBuffer(t *testing.T) {
	payload := []byte("stream payload")
	n, err := Copy(io.Discard, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("copied %d bytes, want %d", n, len(payload))
	}
}

func TestCopyUsesWriterReadFrom(t *testing.T) {
	payload := []byte("stream payload")
	dst := &readerFromWriter{Writer: io.Discard}
	if _, err := Copy(dst, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if !dst.called {
		t.Fatal("expected Copy to delegate to writer ReadFrom")
	}
}

type flushWriter struct {
	io.Writer
	flushes          int
	readerFromCalled bool
}

func (w *flushWriter) Flush() { w.flushes++ }

func (w *flushWriter) ReadFrom(r io.Reader) (int64, error) {
	w.readerFromCalled = true
	return io.Copy(w.Writer, r)
}

func TestCopyFlushAvoidsReaderFromAndFlushesOnce(t *testing.T) {
	payload := []byte("live-audio-chunk")
	dst := &flushWriter{Writer: io.Discard}
	n, err := CopyFlush(dst, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("copied %d, want %d", n, len(payload))
	}
	if dst.readerFromCalled {
		t.Fatal("CopyFlush must not use io.ReaderFrom")
	}
	if dst.flushes != 1 {
		t.Fatalf("flushes=%d, want 1", dst.flushes)
	}
}

// This reader represents an encoder producing small chunks with pauses between
// reads. Each preceding write must be visible before we wait for another chunk.
type pacedAudioReader struct {
	t     *testing.T
	dst   *flushWriter
	reads int
}

func (r *pacedAudioReader) Read(p []byte) (int, error) {
	if r.dst.flushes != r.reads {
		r.t.Fatalf("encoder read %d began with only %d chunks flushed", r.reads+1, r.dst.flushes)
	}
	if r.reads == 3 {
		return 0, io.EOF
	}
	r.reads++
	return copy(p, "audio"), nil
}

func TestCopyFlushDrainsEveryChunkBeforeWaitingForEncoder(t *testing.T) {
	var output bytes.Buffer
	dst := &flushWriter{Writer: &output}
	source := &pacedAudioReader{t: t, dst: dst}
	n, err := CopyFlush(dst, source)
	if err != nil || n != 15 || output.String() != "audioaudioaudio" {
		t.Fatalf("copy = %d, %v, body=%q", n, err, output.String())
	}
}
