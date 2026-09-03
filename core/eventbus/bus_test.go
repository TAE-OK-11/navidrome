package eventbus_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/eventbus"
)

func TestPublishDeliversAsync(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)

	var got atomic.Int32
	done := make(chan struct{})
	bus.Subscribe(eventbus.TopicScrobble, func(ctx context.Context, evt eventbus.Event) {
		if evt.Scrobble == nil || evt.Scrobble.MediaFileID != "mf-1" {
			t.Errorf("unexpected payload %#v", evt.Scrobble)
		}
		got.Add(1)
		close(done)
	})

	bus.Publish(context.Background(), eventbus.Event{
		Topic:    eventbus.TopicScrobble,
		Scrobble: &eventbus.Scrobble{MediaFileID: "mf-1", Title: "Song"},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called")
	}
	if got.Load() != 1 {
		t.Fatalf("got %d deliveries", got.Load())
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)

	var got atomic.Int32
	cancel := bus.Subscribe(eventbus.TopicNowPlaying, func(context.Context, eventbus.Event) {
		got.Add(1)
	})
	cancel()
	bus.PublishSync(context.Background(), eventbus.Event{Topic: eventbus.TopicNowPlaying})
	if got.Load() != 0 {
		t.Fatalf("unsubscribed handler was called")
	}
}

func TestHandlerPanicIsolated(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)

	var got atomic.Int32
	bus.Subscribe(eventbus.TopicPlaybackReport, func(context.Context, eventbus.Event) {
		panic("boom")
	})
	bus.Subscribe(eventbus.TopicPlaybackReport, func(context.Context, eventbus.Event) {
		got.Add(1)
	})
	bus.PublishSync(context.Background(), eventbus.Event{Topic: eventbus.TopicPlaybackReport})
	if got.Load() != 1 {
		t.Fatal("second handler should still run after panic")
	}
}
