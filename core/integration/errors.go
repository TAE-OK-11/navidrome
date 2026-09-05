package integration

import (
	"errors"
	"strings"

	"github.com/navidrome/navidrome/core/rustworker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isWorkerTransportFailure reports whether err indicates the gRPC worker
// process or channel failed. Upstream HTTP failures returned inside a
// successful worker RPC must not kill the worker.
func isWorkerTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	if isWorkerCircuitOpen(err) {
		return false
	}
	if errors.Is(err, errBodyTooLarge) {
		return false
	}
	if strings.HasPrefix(err.Error(), "integration worker: ") {
		return false
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable:
			return true
		case codes.Canceled, codes.DeadlineExceeded, codes.InvalidArgument,
			codes.NotFound, codes.AlreadyExists, codes.PermissionDenied,
			codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented,
			codes.ResourceExhausted, codes.Unauthenticated:
			return false
		default:
			// Internal/Unknown: only kill the worker when the channel looks dead.
			return rustworker.LooksLikeDeadChannel(st.Message())
		}
	}
	return true
}
