package publicgrpc

import (
	"context"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
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
	ip := clientIP(ctx)
	if ip == "" {
		ip = unknownClientIP
	}
	v, _ := loginLimiters.LoadOrStore(ip, rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit))
	if !v.(*rate.Limiter).Allow() {
		log.Warn(ctx, "Public gRPC login rate limit exceeded", "ip", ip)
		return status.Error(codes.ResourceExhausted, "too many login attempts")
	}
	return nil
}
