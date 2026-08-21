package server

import (
	"context"
	"net/http"
)

// http3Service is deliberately request-oriented. The tokio-quiche companion
// owns the UDP/QUIC lifecycle and exposes only HTTP handler integration to the
// Go core.
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
	return newRustHTTP3Runtime(ctx, addr, handler, certFile, keyFile)
}

func clearHTTP3Advertisement(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Alt-Svc", "clear")
		next.ServeHTTP(w, req)
	})
}
