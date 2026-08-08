package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/navidrome/navidrome/conf"
)

// http3Service is deliberately request-oriented. Implementations own their
// UDP/QUIC lifecycle and expose only HTTP handler integration to the Go core.
type http3Service interface {
	serve() error
	advertise(http.Handler) http.Handler
	shutdown(context.Context) error
}

func newConfiguredHTTP3Runtime(
	ctx context.Context,
	addr string,
	handler http.Handler,
	certFile, keyFile string,
) (http3Service, error) {
	switch conf.HTTP3Provider() {
	case conf.HTTP3ProviderQuicGo:
		return newHTTP3Runtime(ctx, addr, handler, certFile, keyFile)
	case conf.HTTP3ProviderTokioQuiche:
		return newRustHTTP3Runtime(ctx, addr, handler, certFile, keyFile)
	default:
		return nil, fmt.Errorf("unsupported HTTP/3 provider %q", conf.HTTP3Provider())
	}
}

func clearHTTP3Advertisement(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Alt-Svc", "clear")
		next.ServeHTTP(w, req)
	})
}
