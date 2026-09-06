package neterr

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
)

func TestIsExpectedClientDisconnect(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "nil", ctx: context.Background(), err: nil, want: false},
		{name: "canceled ctx", ctx: canceled, err: errors.New("write"), want: true},
		{name: "context.Canceled", ctx: context.Background(), err: context.Canceled, want: true},
		{name: "EPIPE", ctx: context.Background(), err: syscall.EPIPE, want: true},
		{name: "closed pipe", ctx: context.Background(), err: io.ErrClosedPipe, want: true},
		{name: "net closed", ctx: context.Background(), err: net.ErrClosed, want: true},
		{name: "broken pipe msg", ctx: context.Background(), err: errors.New("write: broken pipe"), want: true},
		{name: "http2 stream closed", ctx: context.Background(), err: errors.New("http2: stream closed"), want: true},
		{name: "grpc transport", ctx: context.Background(), err: errors.New("transport is closing"), want: true},
		{name: "real failure", ctx: context.Background(), err: errors.New("disk full"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsExpectedClientDisconnect(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("IsExpectedClientDisconnect() = %v, want %v", got, tc.want)
			}
		})
	}
}
