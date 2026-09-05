package ioutils

import (
	"io"
	"os"
	"sync"
)

// DefaultCopyBufferSize matches the largest HTTP/2 bridge frame used by the Rust
// HTTP/3 companion so streaming hot paths can reuse one tuned buffer size.
const DefaultCopyBufferSize = 64 * 1024

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, DefaultCopyBufferSize)
		return &buf
	},
}

type underlyingFile interface {
	UnderlyingFile() *os.File
}

type exitError interface {
	ExitError() error
}

// Copy copies from r to w using sendfile/splice when the writer supports
// io.ReaderFrom and the reader is file-backed; otherwise it prefers the
// writer's ReadFrom implementation, then falls back to a pooled buffer.
func Copy(w io.Writer, r io.Reader) (int64, error) {
	if fr, ok := r.(underlyingFile); ok {
		if file := fr.UnderlyingFile(); file != nil {
			if rf, ok := w.(io.ReaderFrom); ok {
				n, err := rf.ReadFrom(file)
				if err != nil {
					return n, err
				}
				return propagateExitError(r, n)
			}
		}
	}
	if rf, ok := w.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		if err != nil {
			return n, err
		}
		return propagateExitError(r, n)
	}
	bufPtr := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufPtr)
	return io.CopyBuffer(w, r, *bufPtr)
}

func propagateExitError(r io.Reader, n int64) (int64, error) {
	if ec, ok := r.(exitError); ok {
		if waitErr := ec.ExitError(); waitErr != nil {
			return n, waitErr
		}
	}
	return n, nil
}
