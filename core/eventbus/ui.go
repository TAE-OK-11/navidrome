package eventbus

import (
	"context"

	"github.com/navidrome/navidrome/model/request"
)

// UIAttrs copies targeting from ctx so the SSE bridge can honor broadcast vs
// per-user delivery without producers importing server/events.
func UIAttrs(ctx context.Context, broadcast bool) map[string]string {
	attrs := make(map[string]string, 3)
	if broadcast {
		attrs[AttrBroadcast] = "1"
		return attrs
	}
	if u, ok := request.UsernameFrom(ctx); ok && u != "" {
		attrs[AttrUsername] = u
	}
	if id, ok := request.ClientUniqueIdFrom(ctx); ok && id != "" {
		attrs[AttrClientUniqueID] = id
	}
	return attrs
}

func mergeAttrs(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (b *Bus) PublishUI(ctx context.Context, evt Event, broadcast bool) {
	evt.Attrs = mergeAttrs(evt.Attrs, UIAttrs(ctx, broadcast))
	b.Publish(ctx, evt)
}

func (b *Bus) PublishUISync(ctx context.Context, evt Event, broadcast bool) {
	evt.Attrs = mergeAttrs(evt.Attrs, UIAttrs(ctx, broadcast))
	b.PublishSync(ctx, evt)
}
