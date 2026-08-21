//go:build linux

package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// TestRustHTTP3CompanionSmoke is opt-in because it exercises the separately
// built tokio-quiche binary. Protocol-level HTTP/3 coverage lives in the Rust
// crate so the Go module never needs a second QUIC implementation merely as a
// test client.
func TestRustHTTP3CompanionSmoke(t *testing.T) {
	binary := os.Getenv("NAVIDROME_H3_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("NAVIDROME_H3_INTEGRATION_BINARY is not set")
	}

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	address := udp.LocalAddr().String()
	if err := udp.Close(); err != nil {
		t.Fatal(err)
	}

	oldPath := viper.Get("http3gatewaypath")
	viper.Set("http3gatewaypath", binary)
	t.Cleanup(func() { viper.Set("http3gatewaypath", oldPath) })

	certFile := filepath.Join("testdata", "test_cert.pem")
	keyFile := filepath.Join("testdata", "test_key.pem")
	runtimeService, err := newRustHTTP3Runtime(
		t.Context(),
		address,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		certFile,
		keyFile,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimeService.(*rustHTTP3Runtime)
	if !runtime.ready.Load() {
		t.Fatal("tokio-quiche companion did not become ready")
	}

	recorder := httptest.NewRecorder()
	runtime.advertise(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://example.test/ping", nil))
	if got := recorder.Header().Get("Alt-Svc"); !strings.HasPrefix(got, `h3=":`) {
		t.Fatalf("Alt-Svc=%q, expected ready HTTP/3 advertisement", got)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- runtime.serve() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runtime.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("tokio-quiche supervisor stopped with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tokio-quiche supervisor did not stop")
	}
}
