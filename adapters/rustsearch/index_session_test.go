package rustsearch

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/core/searchworker/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sessionSearchClient struct {
	gen.SearchClient
	calls int
	fail  bool
}

func (c *sessionSearchClient) Apply(context.Context, *gen.IndexRequest, ...grpc.CallOption) (*gen.IndexResponse, error) {
	c.calls++
	if c.fail {
		return nil, status.Error(codes.Unavailable, "worker lost")
	}
	return &gen.IndexResponse{Ok: true, Protocol: protocolVersion}, nil
}

func TestIndexSessionNeverReplaysIntoReplacementWorker(t *testing.T) {
	client := &sessionSearchClient{}
	ctx := context.WithValue(context.Background(), indexSessionKey{}, client)
	engine := New()
	if _, err := engine.roundTrip(ctx, request{Op: "upsert"}); err != nil {
		t.Fatal(err)
	}
	client.fail = true
	if _, err := engine.roundTrip(ctx, request{Op: "upsert"}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatalf("failed batch replayed: %d calls", client.calls)
	}
	// Cleanup is pinned too, even when it detaches from caller cancellation.
	cleanup, err := beginIndexSession(context.WithoutCancel(ctx))
	if err != nil || indexSession(cleanup) != client {
		t.Fatalf("cleanup changed worker: %v", err)
	}
	if _, err := engine.roundTrip(cleanup, request{Op: "abort_replace"}); status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Fatalf("cleanup replayed: %d calls", client.calls)
	}
}
