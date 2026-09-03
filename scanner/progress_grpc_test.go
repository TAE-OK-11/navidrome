package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/eventbus"
)

func TestScanProgressGRPCRoundTrip(t *testing.T) {
	bus := eventbus.Get()
	got := make(chan *ProgressInfo, 1)
	unsub := bus.Subscribe(eventbus.TopicScanProgress, func(_ context.Context, evt eventbus.Event) {
		if evt.ScanProgress != nil {
			got <- ProgressFromEvent(evt.ScanProgress)
		}
	})
	t.Cleanup(unsub)

	listener, err := startProgressListener("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(listener.Stop)

	progress := make(chan *ProgressInfo, 1)
	progress <- &ProgressInfo{Path: "/music/a", FileCount: 3, Phase: "folder"}
	close(progress)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ReportProgressGRPC(ctx, listener.addr, progress); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-got:
		if p.Path != "/music/a" || p.FileCount != 3 || p.Phase != "folder" {
			t.Fatalf("unexpected progress: %+v", p)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for scan progress over gRPC")
	}
}
