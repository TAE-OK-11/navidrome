package server

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"

	"github.com/navidrome/navidrome/core/stream/hotcache"
)

func expectedClientDisconnect(ctx context.Context, err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed))
}

// RecordExpectedTransportError records a client disconnect and returns true.
// Unexpected transport errors are recorded separately and return false so the
// caller can retain warning-level logging.
func RecordExpectedTransportError(ctx context.Context, err error, mediaID string) bool {
	if expectedClientDisconnect(ctx, err) {
		hotcache.RecordTransportEvent(true, "expected_broken_pipe", "client_cancelled", mediaID, err.Error())
		return true
	}
	hotcache.RecordTransportEvent(false, "unexpected_broken_pipe", "transport_error", mediaID, err.Error())
	return false
}
