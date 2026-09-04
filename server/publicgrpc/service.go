// Package publicgrpc serves the client-facing gRPC API on the same TCP
// listener as HTTP/1.1 and HTTP/2. Requests with Content-Type application/grpc
// are handed to this server; everything else stays on the REST/UI routers.
package publicgrpc

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/eventbus"
	"github.com/navidrome/navidrome/core/lifecycle"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Invoker runs a Subsonic endpoint in-process. Production uses *subsonic.Router.
type Invoker interface {
	Invoke(ctx context.Context, endpoint string, query url.Values, username string, asJSON bool) (string, []byte, error)
}

// NativeInvoker runs Native REST in-process.
type NativeInvoker interface {
	Invoke(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, token string) (int, http.Header, []byte, error)
}

// Service implements navidrome.public.v1.Public.
type Service struct {
	gen.UnimplementedPublicServer
	ds      model.DataStore
	invoker Invoker
	native  NativeInvoker
	bus     *eventbus.Bus
}

func NewService(ds model.DataStore, invoker Invoker, native NativeInvoker, bus *eventbus.Bus) *Service {
	if bus == nil {
		bus = eventbus.Get()
	}
	return &Service{ds: ds, invoker: invoker, native: native, bus: bus}
}

// publicGRPCMaxMsgBytes caps buffered unary payloads. Invoke/InvokeNative
// proxy Subsonic/Native REST bodies up to 32 MiB, so the gRPC frame budget
// must cover them; streaming Open chunks stay at 64 KiB each.
const publicGRPCMaxMsgBytes = 32 << 20

// NewServer returns a gRPC server multiplexed onto the public HTTP/2 listener
// (and onto the optional plaintext H2C listener for WireGuard origins).
// It enables standard health checks, generous message budgets matching the
// 32 MiB REST proxy cap, and keepalive so long-lived Open/Subscribe streams
// survive NAT / WireGuard idle timeouts. Reflection is opt-in via config.
func NewServer(ds model.DataStore, invoker Invoker, native NativeInvoker) *grpc.Server {
	gs := grpc.NewServer(
		grpc.MaxRecvMsgSize(publicGRPCMaxMsgBytes),
		grpc.MaxSendMsgSize(publicGRPCMaxMsgBytes),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     5 * time.Minute,
			MaxConnectionAge:      0,
			MaxConnectionAgeGrace: 30 * time.Second,
			Time:                  2 * time.Minute,
			Timeout:               20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	gen.RegisterPublicServer(gs, NewService(ds, invoker, native, eventbus.Get()))
	// Standard health service so proxies (Pingola warmup) and orchestrators
	// can probe without credentials: Ping remains the authed liveness check.
	hs := health.NewServer()
	hs.SetServingStatus("navidrome.public.v1.Public", grpc_health_v1.HealthCheckResponse_SERVING)
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(gs, hs)
	if conf.PublicGRPCReflectionEnabled() {
		reflection.Register(gs)
	}
	lifecycle.Register(grpcCloser{gs})
	return gs
}

type grpcCloser struct{ gs *grpc.Server }

func (c grpcCloser) Close() {
	if c.gs != nil {
		c.gs.GracefulStop()
	}
}

func (s *Service) Ping(context.Context, *gen.PingRequest) (*gen.PingResponse, error) {
	return &gen.PingResponse{Ok: true, Version: consts.Version, Protocol: "grpc"}, nil
}

func (s *Service) Invoke(ctx context.Context, req *gen.InvokeRequest) (*gen.InvokeResponse, error) {
	if s.invoker == nil {
		return nil, status.Error(codes.Unavailable, "Subsonic invoker is not configured")
	}
	user, err := s.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	ctx = withUser(ctx, user)
	endpoint := strings.TrimSpace(req.GetEndpoint())
	if endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint is required")
	}
	query := url.Values{}
	for k, v := range req.GetParams() {
		query.Set(k, v)
	}
	ct, body, err := s.invoker.Invoke(ctx, endpoint, query, user.UserName, req.GetJson())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "invoke %s: %v", endpoint, err)
	}
	return &gen.InvokeResponse{ContentType: ct, Body: body, Status: http.StatusOK}, nil
}

func (s *Service) InvokeNative(ctx context.Context, req *gen.InvokeNativeRequest) (*gen.InvokeNativeResponse, error) {
	if s.native == nil {
		return nil, status.Error(codes.Unavailable, "Native API invoker is not configured")
	}
	user, err := s.authenticate(ctx)
	if err != nil {
		return nil, err
	}
	ctx = withUser(ctx, user)
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required")
	}
	query := url.Values{}
	for k, v := range req.GetParams() {
		query.Set(k, v)
	}
	statusCode, header, body, err := s.native.Invoke(ctx, req.GetMethod(), path, query, req.GetContentType(), req.GetBody(), bearerToken(ctx))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "native %s: %v", path, err)
	}
	ct := header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	return &gen.InvokeNativeResponse{Status: int32(statusCode), ContentType: ct, Body: body}, nil
}

func (s *Service) Subscribe(req *gen.SubscribeRequest, stream grpc.ServerStreamingServer[gen.Event]) error {
	user, err := s.authenticate(stream.Context())
	if err != nil {
		return err
	}
	topics := req.GetTopics()
	if len(topics) == 0 {
		topics = defaultSubscribeTopics()
	}
	events := make(chan eventbus.Event, 64)
	var unsubs []func()
	for _, topic := range topics {
		t := eventbus.Topic(topic)
		unsubs = append(unsubs, s.bus.Subscribe(t, func(_ context.Context, evt eventbus.Event) {
			if !eventVisibleTo(evt, user) {
				return
			}
			select {
			case events <- evt:
			default:
				log.Trace("Public gRPC Subscribe dropped event", "topic", topic, "user", user.UserName)
			}
		}))
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt := <-events:
			if err := stream.Send(toPublicEvent(evt)); err != nil {
				return err
			}
		}
	}
}

func defaultSubscribeTopics() []string {
	return []string{
		string(eventbus.TopicScanStatus),
		string(eventbus.TopicRefreshResource),
		string(eventbus.TopicNowPlayingCount),
		string(eventbus.TopicScanCompleted),
		string(eventbus.TopicScanProgress),
		string(eventbus.TopicNowPlaying),
		string(eventbus.TopicScrobble),
		string(eventbus.TopicPlaybackReport),
	}
}

func eventVisibleTo(evt eventbus.Event, user *model.User) bool {
	if user == nil {
		return false
	}
	if evt.Attrs[eventbus.AttrBroadcast] == "1" || evt.Attrs[eventbus.AttrBroadcast] == "true" {
		return true
	}
	if owner := evt.Attrs[eventbus.AttrUsername]; owner != "" {
		return owner == user.UserName
	}
	if evt.Scrobble != nil {
		return evt.Scrobble.Username == user.UserName || evt.Scrobble.UserID == user.ID
	}
	if evt.NowPlaying != nil {
		return evt.NowPlaying.UserID == user.ID
	}
	if evt.Report != nil {
		return evt.Report.UserID == user.ID
	}
	if evt.UIScan != nil || evt.Refresh != nil || evt.Scan != nil || evt.ScanProgress != nil || evt.NowPlayingCount != nil {
		return true
	}
	return false
}

func (s *Service) authenticate(ctx context.Context) (*model.User, error) {
	if !conf.PublicGRPCEnabled() {
		return nil, status.Error(codes.Unavailable, "public gRPC is disabled")
	}
	if s.ds == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
	}
	token := bearerToken(ctx)
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "missing authorization bearer token")
	}
	claims, err := auth.Validate(token)
	if err != nil || claims.Subject == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	user, err := s.ds.User(ctx).FindByUsername(claims.Subject)
	if err != nil {
		log.Warn(ctx, "Public gRPC token user not found", "username", claims.Subject, err)
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return user, nil
}

func bearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, v := range md.Get("authorization") {
		v = strings.TrimSpace(v)
		if len(v) > 7 && strings.EqualFold(v[:6], "bearer") && v[6] == ' ' {
			return strings.TrimSpace(v[7:])
		}
	}
	return ""
}

func withUser(ctx context.Context, user *model.User) context.Context {
	ctx = request.WithUsername(ctx, user.UserName)
	return request.WithUser(ctx, *user)
}
