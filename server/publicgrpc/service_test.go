package publicgrpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMuxHTTP1StaysOnNext(t *testing.T) {
	called := false
	h := Mux(grpc.NewServer(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/rest/ping.view", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("HTTP/1.1 must stay on the REST/UI handler")
	}
}

func TestMuxGRPCHTTP2GoesToServer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/navidrome.public.v1.Public/Ping", nil)
	req.Proto = "HTTP/2.0"
	req.ProtoMajor = 2
	req.Header.Set("Content-Type", "application/grpc")
	if !isGRPCRequest(req) {
		t.Fatal("HTTP/2 application/grpc must be recognized")
	}
}

func TestPingUnauthenticated(t *testing.T) {
	svc := NewService(nil, nil, nil, eventbus.New())
	resp, err := svc.Ping(context.Background(), &gen.PingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Ok || resp.Protocol != "grpc" {
		t.Fatalf("ping=%+v", resp)
	}
}

func TestInvokeRequiresAuth(t *testing.T) {
	conf.SetPublicGRPCEnabledForTest(true)
	svc := NewService(nil, stubInvoker{}, nil, eventbus.New())
	_, err := svc.Invoke(context.Background(), &gen.InvokeRequest{Endpoint: "ping"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestInvokeNativeRequiresAuth(t *testing.T) {
	conf.SetPublicGRPCEnabledForTest(true)
	svc := NewService(nil, nil, stubNativeInvoker{}, eventbus.New())
	_, err := svc.InvokeNative(context.Background(), &gen.InvokeNativeRequest{Path: "/song"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestSubscribeRequiresAuth(t *testing.T) {
	conf.SetPublicGRPCEnabledForTest(true)
	svc := NewService(nil, nil, nil, eventbus.New())
	err := svc.Subscribe(&gen.SubscribeRequest{}, stubSubscribeStream{ctx: context.Background()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestEventVisibleTo(t *testing.T) {
	broadcast := eventbus.Event{Attrs: map[string]string{eventbus.AttrBroadcast: "1"}}
	if !eventVisibleTo(broadcast, &model.User{UserName: "bob"}) {
		t.Fatal("broadcast events must reach every subscriber")
	}
	bob := &model.User{ID: "u2", UserName: "bob"}
	if eventVisibleTo(eventbus.Event{Attrs: map[string]string{eventbus.AttrUsername: "alice"}}, bob) {
		t.Fatal("per-user attr events must not leak")
	}
	if !eventVisibleTo(eventbus.Event{Attrs: map[string]string{eventbus.AttrUsername: "bob"}}, bob) {
		t.Fatal("owner must see their events")
	}
	if eventVisibleTo(eventbus.Event{
		NowPlaying: &eventbus.NowPlaying{UserID: "u1"},
	}, bob) {
		t.Fatal("now playing must not leak across users")
	}
	if !eventVisibleTo(eventbus.Event{
		Scrobble: &eventbus.Scrobble{UserID: "u2", Username: "bob"},
	}, bob) {
		t.Fatal("scrobble owner must see their events")
	}
	if eventVisibleTo(eventbus.Event{
		Report: &eventbus.PlaybackReport{UserID: "u1"},
	}, bob) {
		t.Fatal("playback report must not leak across users")
	}
	if !eventVisibleTo(eventbus.Event{
		UIScan: &eventbus.UIScanStatus{Scanning: true},
	}, bob) {
		t.Fatal("global UI scan events should be visible")
	}
}

func TestBearerToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer abc"))
	if got := bearerToken(ctx); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestToPublicEventScanStatus(t *testing.T) {
	evt := eventbus.Event{
		ID:     "e1",
		Topic:  eventbus.TopicScanStatus,
		UIScan: &eventbus.UIScanStatus{Scanning: true, Count: 3},
	}
	out := toPublicEvent(evt)
	if out.GetScanStatus() == nil || !out.GetScanStatus().Scanning || out.GetScanStatus().Count != 3 {
		t.Fatalf("scan status payload=%v", out.GetPayload())
	}
}

type stubInvoker struct{}

func (stubInvoker) Invoke(context.Context, string, url.Values, string, bool) (int, string, []byte, error) {
	return http.StatusOK, "application/json", []byte(`{"ok":true}`), nil
}

type stubNativeInvoker struct{}

func (stubNativeInvoker) Invoke(context.Context, string, string, url.Values, string, []byte, string) (int, http.Header, []byte, error) {
	return http.StatusOK, http.Header{"Content-Type": []string{"application/json"}}, []byte(`[]`), nil
}

type stubSubscribeStream struct {
	grpc.ServerStreamingServer[gen.Event]
	ctx context.Context
}

func (s stubSubscribeStream) Context() context.Context { return s.ctx }
func (s stubSubscribeStream) Send(*gen.Event) error    { return io.EOF }
