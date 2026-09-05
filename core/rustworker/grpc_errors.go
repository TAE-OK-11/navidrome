package rustworker

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsTransportFailure reports whether err indicates the gRPC worker process or
// channel failed. Application-level errors returned inside a successful RPC,
// and client-side cancellation/deadlines, must not trigger a worker restart.
func IsTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Canceled, codes.DeadlineExceeded, codes.InvalidArgument,
			codes.NotFound, codes.AlreadyExists, codes.PermissionDenied,
			codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented,
			codes.ResourceExhausted, codes.Unauthenticated:
			return false
		case codes.Unavailable:
			return true
		default:
			// Internal/Unknown can be application faults; only treat them as
			// transport failures when the message looks like a dead channel.
			return LooksLikeDeadChannel(st.Message())
		}
	}
	// Non-gRPC errors from the client stack (broken pipe, connection reset, etc.).
	return LooksLikeDeadChannel(err.Error())
}

// LooksLikeDeadChannel reports whether a free-form error message describes a
// dead gRPC transport rather than an application failure.
func LooksLikeDeadChannel(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "transport is closing") ||
		strings.Contains(msg, "error reading from server") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "server closed the stream without sending trailers")
}
