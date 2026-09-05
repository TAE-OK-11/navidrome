package rustworker

import (
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsTransportFailure reports whether err indicates the gRPC worker process or
// channel failed. Application-level errors returned inside a successful RPC
// must not trigger a worker restart.
func IsTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown:
			return true
		default:
			return false
		}
	}
	// Non-gRPC errors from the client stack (broken pipe, connection reset, etc.).
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "transport is closing") {
		return true
	}
	return errors.Is(err, ErrSkippedInTests)
}
