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

const loginLimiterMapLimit = 4096

type loginLimiterEntry struct {
	limiter *rate.Limiter
	lastUse time.Time
}

type loginLimiterStore struct {
	mu      sync.Mutex
	entries map[string]loginLimiterEntry
	limit   int
}

var loginLimiters loginLimiterStore

func init() {
	loginLimiters = loginLimiterStore{entries: make(map[string]loginLimiterEntry), limit: loginLimiterMapLimit}
}

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
	limiter := loginLimiters.get(ip, limit, window)
	if !limiter.Allow() {
		log.Warn(ctx, "Public gRPC login rate limit exceeded", "ip", ip)
		return status.Error(codes.ResourceExhausted, "too many login attempts")
	}
	return nil
}

func (s *loginLimiterStore) get(ip string, requestLimit int, window time.Duration) *rate.Limiter {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.entries[ip]; ok {
		entry.lastUse = now
		s.entries[ip] = entry
		return entry.limiter
	}
	if len(s.entries) >= s.limit {
		s.evict(now)
	}
	limiter := rate.NewLimiter(rate.Every(window/time.Duration(requestLimit)), requestLimit)
	s.entries[ip] = loginLimiterEntry{limiter: limiter, lastUse: now}
	return limiter
}

func (s *loginLimiterStore) evict(now time.Time) {
	var oldestKey string
	var oldestUse time.Time
	for key, entry := range s.entries {
		if oldestKey == "" || entry.lastUse.Before(oldestUse) {
			oldestKey, oldestUse = key, entry.lastUse
		}
	}
	if oldestKey != "" {
		delete(s.entries, oldestKey)
	}
}
