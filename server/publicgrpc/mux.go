package publicgrpc

import (
	"net/http"
	"strings"

	"google.golang.org/grpc"
)

// Mux routes HTTP/2 gRPC requests to gs and every other request to next.
// HTTP/1.1, REST, and the UI keep their existing handlers. The HTTP/3
// companion already converts QUIC streams to HTTP/2 before this mux, so
// application/grpc over H3 reaches the same server.
func Mux(gs *grpc.Server, next http.Handler) http.Handler {
	if gs == nil {
		return next
	}
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGRPCRequest(r) {
			gs.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GRPCOnly serves the public gRPC API over H2C and rejects REST/UI traffic.
// Use on the dedicated plaintext WireGuard listener so the main TLS port
// remains the only REST entrypoint.
func GRPCOnly(gs *grpc.Server) http.Handler {
	if gs == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "gRPC is not configured", http.StatusServiceUnavailable)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isGRPCRequest(r) {
			gs.ServeHTTP(w, r)
			return
		}
		http.Error(w, "plaintext H2C listener accepts gRPC only; use the main port for REST", http.StatusForbidden)
	})
}

func isGRPCRequest(r *http.Request) bool {
	if r == nil || r.ProtoMajor < 2 {
		return false
	}
	ct := r.Header.Get("Content-Type")
	return strings.HasPrefix(ct, "application/grpc")
}
