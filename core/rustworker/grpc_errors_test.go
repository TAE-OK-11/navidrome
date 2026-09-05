package rustworker

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsTransportFailure(t *testing.T) {
	if IsTransportFailure(nil) {
		t.Fatal("nil should not be transport failure")
	}
	if !IsTransportFailure(status.Error(codes.Unavailable, "down")) {
		t.Fatal("Unavailable should be transport failure")
	}
	if IsTransportFailure(status.Error(codes.InvalidArgument, "bad")) {
		t.Fatal("InvalidArgument should not be transport failure")
	}
	if !IsTransportFailure(errors.New("connection reset by peer")) {
		t.Fatal("connection reset should be transport failure")
	}
}
