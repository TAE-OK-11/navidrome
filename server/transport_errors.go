package server

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
)

func expectedClientDisconnect(ctx context.Context, err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) || errors.Is(err, os.ErrClosed))
}

// IsExpectedTransportError reports whether a response ended because the
// client disconnected. Callers can keep normal disconnects out of warning logs.
func IsExpectedTransportError(ctx context.Context, err error) bool {
	return expectedClientDisconnect(ctx, err)
}
