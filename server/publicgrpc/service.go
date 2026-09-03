// Package publicgrpc serves the client-facing gRPC API on the same TCP
// listener as HTTP/1.1 and HTTP/2. Requests with Content-Type application/grpc
// are handed to this server; everything else stays on the REST/UI routers.
package publicgrpc

import (
	"context"
	"net/url"
	"strings"

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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Invoker runs a Subsonic endpoint in-process. Production uses *subsonic.Router.
type Invoker interface {
	Invoke(ctx context.Context, endpoint string, query url.Values, username string, asJSON bool) (string, []byte, error)
}

// Service implements navidrome.public.v1.Public.
type Service struct {
	gen.UnimplementedPublicServer
	ds      model.DataStore
	invoker Invoker
	bus     *eventbus.Bus
}

func NewService(ds model.DataStore, invoker Invoker, bus *eventbus.Bus) *Service {
	if bus == nil {
		bus = eventbus.Get()
	}
	return &Service{ds: ds, invoker: invoker, bus: bus}
}

// NewServer returns a gRPC server multiplexed onto the public HTTP/2 listener.
func NewServer(ds model.DataStore, invoker Invoker) *grpc.Server {
	gs := grpc.NewServer()
	gen.RegisterPublicServer(gs, NewService(ds, invoker, eventbus.Get()))
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
	return &gen.InvokeResponse{ContentType: ct, Body: body}, nil
}

func (s *Service) Subscribe(req *gen.SubscribeRequest, stream grpc.ServerStreamingServer[gen.Event]) error {
	user, err := s.authenticate(stream.Context())
	if err != nil {
		return err
	}
	topics := req.GetTopics()
	if len(topics) == 0 {
		topics = []string{
			string(eventbus.TopicScanStatus),
			string(eventbus.TopicRefreshResource),
			string(eventbus.TopicNowPlayingCount),
			string(eventbus.TopicScanCompleted),
		}
	}
	events := make(chan eventbus.Event, 64)
	var unsubs []func()
	for _, topic := range topics {
		t := eventbus.Topic(topic)
		unsubs = append(unsubs, s.bus.Subscribe(t, func(_ context.Context, evt eventbus.Event) {
			if !eventVisibleTo(evt, user.UserName) {
				return
			}
			select {
			case events <- evt:
			default:
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

func eventVisibleTo(evt eventbus.Event, username string) bool {
	if evt.Attrs[eventbus.AttrBroadcast] == "1" || evt.Attrs[eventbus.AttrBroadcast] == "true" {
		return true
	}
	owner := evt.Attrs[eventbus.AttrUsername]
	return owner == "" || owner == username
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
		if strings.HasPrefix(strings.ToLower(v), "bearer ") {
			return strings.TrimSpace(v[7:])
		}
		if v != "" {
			return v
		}
	}
	if vals := md.Get("token"); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func withUser(ctx context.Context, user *model.User) context.Context {
	ctx = request.WithUsername(ctx, user.UserName)
	return request.WithUser(ctx, *user)
}
