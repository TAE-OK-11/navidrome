package publicgrpc

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/ipallowlist"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type listenerKindKey struct{}

const unknownClientIP = "unknown-peer"

func withListenerKind(kind string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), listenerKindKey{}, kind)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func listenerKind(ctx context.Context) string {
	kind, _ := ctx.Value(listenerKindKey{}).(string)
	return kind
}

func networkUnaryInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := checkClientNetwork(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func networkStreamInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := checkClientNetwork(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

func checkClientNetwork(ctx context.Context) error {
	allowed := conf.PublicGRPCAllowedIPs()
	if allowed == "" {
		return nil
	}
	ip := clientIP(ctx)
	if ip == "" {
		log.Warn(ctx, "Public gRPC request blocked: client IP unavailable with allowlist enabled")
		return status.Error(codes.PermissionDenied, "client IP not allowed")
	}
	if !ipallowlist.Contains(ip, allowed, false) {
		log.Warn(ctx, "Public gRPC request blocked by IP allowlist", "clientIP", ip, "listener", listenerKind(ctx))
		return status.Error(codes.PermissionDenied, "client IP not allowed")
	}
	return nil
}

func clientIP(ctx context.Context) string {
	peerIP := peerHost(ctx)
	trusted := conf.PublicGRPCTrustedProxies()
	if trusted != "" && peerIP != "" && ipallowlist.Contains(peerIP, trusted, false) {
		if forwarded := forwardedClientIP(ctx); forwarded != "" {
			return forwarded
		}
	}
	return peerIP
}

func peerHost(ctx context.Context) string {
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

func forwardedClientIP(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{"x-forwarded-for", "grpcgateway-x-forwarded-for"} {
		for _, raw := range md.Get(key) {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			client, _, _ := strings.Cut(raw, ",")
			client = strings.TrimSpace(client)
			if net.ParseIP(client) != nil {
				return client
			}
		}
	}
	return ""
}
