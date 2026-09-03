package eventbus_test

import (
	"context"
	"testing"

	"github.com/navidrome/navidrome/core/eventbus"
)

func TestPublishUISyncBroadcast(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)

	var got string
	bus.Subscribe(eventbus.TopicRefreshResource, func(_ context.Context, evt eventbus.Event) {
		if evt.Attrs[eventbus.AttrBroadcast] != "1" {
			t.Errorf("expected broadcast attr")
		}
		if evt.Refresh != nil && len(evt.Refresh.Resources["library"]) > 0 {
			got = evt.Refresh.Resources["library"][0]
		}
	})
	bus.PublishUISync(context.Background(), eventbus.Event{
		Topic:   eventbus.TopicRefreshResource,
		Refresh: (&eventbus.RefreshResource{}).Add("library", "9"),
	}, true)
	if got != "9" {
		t.Fatalf("got %q", got)
	}
}
