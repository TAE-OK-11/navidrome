package rustworker

import (
	"context"
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
	if IsTransportFailure(status.Error(codes.DeadlineExceeded, "timeout")) {
		t.Fatal("DeadlineExceeded should not invalidate the worker")
	}
	if IsTransportFailure(status.Error(codes.Canceled, "canceled")) {
		t.Fatal("Canceled should not invalidate the worker")
	}
	if IsTransportFailure(status.Error(codes.Internal, "tag parse failed")) {
		t.Fatal("application Internal should not invalidate the worker")
	}
	if !IsTransportFailure(status.Error(codes.Internal, "transport is closing")) {
		t.Fatal("Internal with dead-transport message should invalidate")
	}
	if IsTransportFailure(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should not invalidate the worker")
	}
	if !IsTransportFailure(errors.New("connection reset by peer")) {
		t.Fatal("connection reset should be transport failure")
	}
}
