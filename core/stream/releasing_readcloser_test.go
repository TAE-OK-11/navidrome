package stream

import (
	"io"
	"os"
	"testing"

	"github.com/navidrome/navidrome/utils/ioutils"
)

type testPipeReader struct {
	*os.File
	exitErr error
}

func (t testPipeReader) UnderlyingFile() *os.File { return t.File }

func (t testPipeReader) ExitError() error { return t.exitErr }

func (t testPipeReader) Close() error { return t.File.Close() }

type readerFromWriter struct {
	io.Writer
	called bool
}

func (w *readerFromWriter) ReadFrom(r io.Reader) (int64, error) {
	w.called = true
	return io.Copy(w.Writer, r)
}

func TestReleasingReadCloserForwardsUnderlyingFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	released := false
	wrapped := &releasingReadCloser{
		ReadCloser: testPipeReader{File: r},
		release:    func() { released = true },
	}

	dst := &readerFromWriter{Writer: io.Discard}
	if _, err := ioutils.Copy(dst, wrapped); err != nil {
		t.Fatal(err)
	}
	if !dst.called {
		t.Fatal("expected Copy to use io.ReaderFrom via forwarded UnderlyingFile")
	}
	if err := wrapped.Close(); err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("expected limiter release on Close")
	}
}
