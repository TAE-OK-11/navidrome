package publicgrpc

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var loginLimiters sync.Map

func checkLoginRateLimit(ctx context.Context) error {
	limit := conf.Server.AuthRequestLimit
	if limit <= 0 {
		return nil
	}
	window := conf.Server.AuthWindowLength
	if window <= 0 {
		window = time.Minute
	}
	ip := peerIP(ctx)
	if ip == "" {
		return nil
	}
	v, _ := loginLimiters.LoadOrStore(ip, rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit))
	if !v.(*rate.Limiter).Allow() {
		log.Warn(ctx, "Public gRPC login rate limit exceeded", "ip", ip)
		return status.Error(codes.ResourceExhausted, "too many login attempts")
	}
	return nil
}

func peerIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return p.Addr.String()
	}
	return host
}
