package events

import (
	"context"
	"net/http"
	"testing"

	"github.com/navidrome/navidrome/core/eventbus"
)

func TestForwardFromBusRefreshBroadcast(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)
	broker := &recordingBroker{}
	unsub := ForwardFromBus(bus, broker)
	t.Cleanup(unsub)

	refresh := (&eventbus.RefreshResource{}).Add("library", "1")
	bus.PublishUISync(context.Background(), eventbus.Event{
		Topic:   eventbus.TopicRefreshResource,
		Refresh: refresh,
	}, true)

	if len(broker.broadcasts) != 1 {
		t.Fatalf("got %d broadcasts", len(broker.broadcasts))
	}
	rr, ok := broker.broadcasts[0].(*RefreshResource)
	if !ok {
		t.Fatalf("got %T", broker.broadcasts[0])
	}
	if rr.Data(rr) != `{"library":["1"]}` {
		t.Fatalf("payload %s", rr.Data(rr))
	}
}

func TestForwardFromBusNowPlayingCount(t *testing.T) {
	bus := eventbus.NewWithSize(8, 1)
	t.Cleanup(bus.Close)
	broker := &recordingBroker{}
	unsub := ForwardFromBus(bus, broker)
	t.Cleanup(unsub)

	bus.PublishUISync(context.Background(), eventbus.Event{
		Topic:           eventbus.TopicNowPlayingCount,
		NowPlayingCount: &eventbus.UINowPlayingCount{Count: 3},
	}, true)

	if len(broker.broadcasts) != 1 {
		t.Fatalf("got %d broadcasts", len(broker.broadcasts))
	}
	np, ok := broker.broadcasts[0].(*NowPlayingCount)
	if !ok {
		t.Fatalf("got %T", broker.broadcasts[0])
	}
	if np.Count != 3 {
		t.Fatalf("count %d", np.Count)
	}
}

type recordingBroker struct {
	broadcasts []Event
	messages   []Event
}

func (r *recordingBroker) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (r *recordingBroker) SendMessage(_ context.Context, event Event) {
	r.messages = append(r.messages, event)
}
func (r *recordingBroker) SendBroadcastMessage(_ context.Context, event Event) {
	r.broadcasts = append(r.broadcasts, event)
}
