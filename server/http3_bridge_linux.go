//go:build linux

package server

import (
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/navidrome/navidrome/utils/ioutils"
)

// inheritedConnListener lets net/http retain its mature HTTP/2 lifecycle and
// graceful-shutdown behavior while accepting only connections that Navidrome
// itself creates and passes to the tokio-quiche child with AF_UNIX socketpair.
// There is no bind/connect/accept path visible outside the process tree.
type inheritedConnListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newInheritedConnListener() *inheritedConnListener {
	return &inheritedConnListener{
		conns: make(chan net.Conn, 1),
		done:  make(chan struct{}),
	}
}

func (l *inheritedConnListener) add(conn net.Conn) error {
	select {
	case <-l.done:
		return net.ErrClosed
	case l.conns <- conn:
		return nil
	}
}

func (l *inheritedConnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case conn := <-l.conns:
		if conn == nil {
			return nil, errors.New("nil inherited HTTP/3 bridge connection")
		}
		return conn, nil
	}
}

func (l *inheritedConnListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *inheritedConnListener) Addr() net.Addr {
	return inheritedBridgeAddr{}
}

type inheritedBridgeAddr struct{}

func (inheritedBridgeAddr) Network() string { return "unix" }
func (inheritedBridgeAddr) String() string  { return "navidrome-h3-inherited" }

// bridgeFrameWriter batches media responses to the largest HTTP/2 frame size
// used by the Rust HTTP/3 companion so transcoded audio does not fragment the
// inherited bridge with many small DATA frames.
type bridgeFrameWriter struct {
	http.ResponseWriter
	buf []byte
}

func newBridgeFrameWriter(w http.ResponseWriter, path string) *bridgeFrameWriter {
	if !isMediaResponsePath(path) {
		return nil
	}
	return &bridgeFrameWriter{
		ResponseWriter: w,
		buf:          make([]byte, 0, ioutils.DefaultCopyBufferSize),
	}
}

func (w *bridgeFrameWriter) flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.buf)
	w.buf = w.buf[:0]
	return err
}

func (w *bridgeFrameWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		if cap(w.buf)-len(w.buf) == 0 {
			if err := w.flush(); err != nil {
				return total - len(p), err
			}
		}
		n := copy(w.buf[len(w.buf):cap(w.buf)], p)
		w.buf = w.buf[:len(w.buf)+n]
		p = p[n:]
		if len(w.buf) >= ioutils.DefaultCopyBufferSize {
			if err := w.flush(); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}

func (w *bridgeFrameWriter) ReadFrom(source io.Reader) (int64, error) {
	if err := w.flush(); err != nil {
		return 0, err
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(source)
	}
	bufPtr := make([]byte, ioutils.DefaultCopyBufferSize)
	var total int64
	for {
		n, err := source.Read(bufPtr)
		if n > 0 {
			written, writeErr := w.Write(bufPtr[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}
