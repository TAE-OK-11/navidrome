package neterr

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
)

// IsExpectedClientDisconnect reports whether err (or ctx) indicates the peer
// closed the connection. Used to keep normal H1/H2/H3/gRPC disconnects out of
// warning logs and to avoid treating them as server failures after headers.
func IsExpectedClientDisconnect(ctx context.Context, err error) bool {
	if err == nil {
		return ctx != nil && ctx.Err() != nil
	}
	if ctx != nil && ctx.Err() != nil {
		return true
	}

	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, io.ErrClosedPipe),
		errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, net.ErrClosed),
		errors.Is(err, os.ErrClosed),
		errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.ECONNABORTED):
		return true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return IsExpectedClientDisconnect(ctx, opErr.Err)
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection aborted") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "stream closed") ||
		strings.Contains(msg, "stream reset") ||
		strings.Contains(msg, "http2: stream closed") ||
		strings.Contains(msg, "client disconnected") ||
		strings.Contains(msg, "client gone") ||
		strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "cancelled") ||
		strings.Contains(msg, "transport is closing") ||
		strings.Contains(msg, "connection is closing") ||
		strings.Contains(msg, "server closed") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "error reading from server") ||
		msg == "eof" ||
		strings.HasSuffix(msg, ": eof")
}
