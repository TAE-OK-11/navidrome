package scanner

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/navidrome/navidrome/core/scannerworker/gen"
	"google.golang.org/grpc"
)

type contractWalkClient struct {
	gen.ScannerClient
	events []*gen.WalkEvent
	ctx    context.Context
}

func (c *contractWalkClient) Walk(ctx context.Context, _ *gen.WalkRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[gen.WalkEvent], error) {
	c.ctx = ctx
	return &contractWalkStream{events: c.events}, nil
}

type contractWalkStream struct {
	grpc.ClientStream
	events []*gen.WalkEvent
}

func (s *contractWalkStream) Recv() (*gen.WalkEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}

func TestWalkRequiresDoneAndCancelsStream(t *testing.T) {
	for _, tc := range []struct {
		name    string
		events  []*gen.WalkEvent
		wantErr bool
	}{
		{name: "truncated", wantErr: true},
		{name: "partial", events: []*gen.WalkEvent{{Kind: gen.WalkEventKind_WALK_EVENT_KIND_FOLDER, Folder: &gen.WalkFolder{Path: "album"}}}, wantErr: true},
		{name: "done", events: []*gen.WalkEvent{{Kind: gen.WalkEventKind_WALK_EVENT_KIND_DONE}}},
		{name: "invalid", events: []*gen.WalkEvent{{Kind: gen.WalkEventKind_WALK_EVENT_KIND_FOLDER}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &contractWalkClient{events: tc.events}
			_, err := streamRustFoldersGRPCOnce(context.Background(), client, rustScanRequest{}, make(chan *rustScanFolder, 2))
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if client.ctx.Err() != context.Canceled {
				t.Fatal("stream left running")
			}
			if tc.name == "partial" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal(err)
			}
		})
	}
}

func TestFailedTraversalDoesNotMarkMissing(t *testing.T) {
	want := errors.New("incomplete scan")
	// A nil datastore deliberately catches any attempt to begin cleanup.
	p := &phaseFolders{ctx: context.Background()}
	if err := p.finalize(want); !errors.Is(err, want) {
		t.Fatal(err)
	}
	job := &scanJob{}
	job.setWalkError(want)
	p.jobs = []*scanJob{job}
	if err := p.finalize(nil); !errors.Is(err, want) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.ctx = ctx
	if err := p.finalize(nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
