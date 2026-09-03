package events

import (
	"context"

	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/model/request"
)

// ForwardFromBus maps domain UI topics onto this SSE broker. Producers publish
// on the event bus; the public EventStream stays SSE. Returns unsubscribe.
func ForwardFromBus(bus *eventbus.Bus, broker Broker) func() {
	if bus == nil || broker == nil {
		return func() {}
	}
	unsubRefresh := bus.Subscribe(eventbus.TopicRefreshResource, func(ctx context.Context, evt eventbus.Event) {
		if evt.Refresh == nil {
			return
		}
		sendUI(ctx, broker, evt, &RefreshResource{resources: cloneResources(evt.Refresh.Resources)})
	})
	unsubScan := bus.Subscribe(eventbus.TopicScanStatus, func(ctx context.Context, evt eventbus.Event) {
		if evt.UIScan == nil {
			return
		}
		sendUI(ctx, broker, evt, &ScanStatus{
			Scanning:    evt.UIScan.Scanning,
			Count:       evt.UIScan.Count,
			FolderCount: evt.UIScan.FolderCount,
			Error:       evt.UIScan.Error,
			ScanType:    evt.UIScan.ScanType,
			ElapsedTime: evt.UIScan.ElapsedTime,
		})
	})
	unsubNP := bus.Subscribe(eventbus.TopicNowPlayingCount, func(ctx context.Context, evt eventbus.Event) {
		if evt.NowPlayingCount == nil {
			return
		}
		sendUI(ctx, broker, evt, &NowPlayingCount{Count: evt.NowPlayingCount.Count})
	})
	return func() {
		unsubRefresh()
		unsubScan()
		unsubNP()
	}
}

func sendUI(ctx context.Context, broker Broker, evt eventbus.Event, sse Event) {
	ctx = senderContext(ctx, evt)
	if evt.Attrs[eventbus.AttrBroadcast] == "1" {
		broker.SendBroadcastMessage(ctx, sse)
		return
	}
	broker.SendMessage(ctx, sse)
}

func senderContext(ctx context.Context, evt eventbus.Event) context.Context {
	if u := evt.Attrs[eventbus.AttrUsername]; u != "" {
		ctx = request.WithUsername(ctx, u)
	}
	if id := evt.Attrs[eventbus.AttrClientUniqueID]; id != "" {
		ctx = request.WithClientUniqueId(ctx, id)
	}
	return ctx
}

func cloneResources(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}
