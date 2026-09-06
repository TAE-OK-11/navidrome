package stream

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

type fileWrapper struct {
	*os.File
}

func (w fileWrapper) UnderlyingFile() *os.File { return w.File }

func TestUnderlyingSeekableFilePreferosFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.flac")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if got := underlyingSeekableFile(f); got != f {
		t.Fatalf("direct *os.File unwrap failed")
	}
	wrapped := fileWrapper{File: f}
	if got := underlyingSeekableFile(wrapped); got != f {
		t.Fatalf("UnderlyingFile unwrap failed")
	}
	if got := underlyingSeekableFile(io.NopCloser(f)); got != nil {
		t.Fatalf("expected nil for non-file wrapper, got %v", got)
	}
}
