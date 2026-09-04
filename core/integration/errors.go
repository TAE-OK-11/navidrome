package integration

import (
	"errors"
	"strings"

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
		case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown:
			return true
		default:
			return false
		}
	}
	return true
}
