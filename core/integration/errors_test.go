package integration

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsWorkerTransportFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"circuit open", workerResponseError("circuit open for lastfm"), false},
		{"upstream failure", workerResponseError("connection refused"), false},
		{"body too large", fmt.Errorf("%w (8 bytes)", errBodyTooLarge), false},
		{"grpc unavailable", status.Error(codes.Unavailable, "transport"), true},
		{"grpc internal", status.Error(codes.Internal, "worker died"), true},
		{"client closed", errors.New("integration gRPC client closed"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWorkerTransportFailure(tc.err); got != tc.want {
				t.Fatalf("isWorkerTransportFailure(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}
