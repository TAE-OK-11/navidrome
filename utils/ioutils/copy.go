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

// CopyFlush copies from r to w with a pooled buffer and never uses
// io.ReaderFrom. Live/transcoded pipes must not go through ReadFrom: the
// HTTP/2 stack buffers until a full DATA frame, delaying TTFB and seeks.
// After the first successful write, Flush is called when available so
// clients (and H3 bridges) see bytes immediately.
func CopyFlush(w io.Writer, r io.Reader) (int64, error) {
	bufPtr := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(bufPtr)
	n, err := copyFlushBuffer(w, r, *bufPtr)
	if err != nil {
		return n, err
	}
	return propagateExitError(r, n)
}

type flusher interface {
	Flush()
}

func copyFlushBuffer(w io.Writer, r io.Reader, buf []byte) (int64, error) {
	var written int64
	flushed := false
	for {
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
			if !flushed {
				if f, ok := w.(flusher); ok {
					f.Flush()
				}
				flushed = true
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}
