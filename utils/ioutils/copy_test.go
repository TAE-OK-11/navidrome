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
