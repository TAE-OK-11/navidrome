package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/eventbus"
)

func TestPublishProgressReachesSubscriber(t *testing.T) {
	progress, unsub := SubscribeProgress(eventbus.Get())
	t.Cleanup(unsub)

	go func() {
		PublishProgress(context.Background(), &ProgressInfo{Path: "/music/a", FileCount: 2, Phase: "folder"})
	}()

	select {
	case info := <-progress:
		if info.Path != "/music/a" || info.FileCount != 2 || info.Phase != "folder" {
			t.Fatalf("unexpected progress %#v", info)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive scan progress on the event bus")
	}
}

func TestProgressProtoRoundTrip(t *testing.T) {
	in := &ProgressInfo{
		LibID: 4, FileCount: 9, Path: "x", Phase: "missing",
		ChangesDetected: true, Warning: "w", Error: "e", ForceUpdate: true,
	}
	out := progressFromProto(progressToProto(in))
	if out.LibID != in.LibID || out.FileCount != in.FileCount || out.Path != in.Path ||
		out.Phase != in.Phase || out.ChangesDetected != in.ChangesDetected ||
		out.Warning != in.Warning || out.Error != in.Error || out.ForceUpdate != in.ForceUpdate {
		t.Fatalf("proto round trip mismatch: %#v vs %#v", in, out)
	}
}
