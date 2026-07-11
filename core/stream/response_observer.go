package stream

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"
)

// observedResponseWriter preserves io.ReaderFrom so net/http can still reach
// its sendfile path. It records only request-local scalar data and is created
// exclusively for Hot Cache enabled direct streams.
type observedResponseWriter struct {
	http.ResponseWriter
	started        time.Time
	firstWrite     time.Time
	bytes          int64
	writeErr       error
	readerFromUsed bool
	readerFromFast bool
	once           sync.Once
}

func newObservedResponseWriter(w http.ResponseWriter) *observedResponseWriter {
	return &observedResponseWriter{ResponseWriter: w, started: time.Now()}
}

func (w *observedResponseWriter) markFirstWrite() {
	w.once.Do(func() { w.firstWrite = time.Now() })
}

func (w *observedResponseWriter) WriteHeader(status int) {
	w.markFirstWrite()
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedResponseWriter) Write(data []byte) (int, error) {
	w.markFirstWrite()
	count, err := w.ResponseWriter.Write(data)
	w.bytes += int64(count)
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	return count, err
}

func (w *observedResponseWriter) ReadFrom(source io.Reader) (int64, error) {
	w.markFirstWrite()
	w.readerFromUsed = true
	readerFrom, ok := w.ResponseWriter.(io.ReaderFrom)
	w.readerFromFast = ok
	var count int64
	var err error
	if ok {
		count, err = readerFrom.ReadFrom(source)
	} else {
		count, err = io.Copy(w.ResponseWriter, source)
	}
	w.bytes += count
	if err != nil && w.writeErr == nil {
		w.writeErr = err
	}
	return count, err
}

func (w *observedResponseWriter) Flush() {
	w.markFirstWrite()
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *observedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *observedResponseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (w *observedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *observedResponseWriter) ttfb() time.Duration {
	if w.firstWrite.IsZero() {
		return time.Since(w.started)
	}
	return w.firstWrite.Sub(w.started)
}

func expectedClientDisconnect(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed))
}
