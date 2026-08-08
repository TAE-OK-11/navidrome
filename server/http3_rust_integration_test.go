//go:build linux

package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/spf13/viper"
)

// TestRustHTTP3EndToEnd is opt-in because it exercises the separately built
// Rust binary. CI and release builds can enable it with
// NAVIDROME_H3_INTEGRATION_BINARY=/path/to/navidrome-h3.
func TestRustHTTP3EndToEnd(t *testing.T) {
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

	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.ProtoMajor != 3 || req.TLS == nil {
			http.Error(w, "outer transport metadata missing", http.StatusInternalServerError)
			return
		}
		switch req.URL.Path {
		case "/range":
			if req.Header.Get("Range") != "bytes=4-9" {
				http.Error(w, "range header missing", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Range", "bytes 4-9/16")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "456789")
		case "/echo":
			_, _ = io.Copy(w, req.Body)
		case "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: one\n\ndata: two\n\n")
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	certFile := filepath.Join("testdata", "test_cert.pem")
	keyFile := filepath.Join("testdata", "test_key.pem")
	runtimeService, err := newRustHTTP3Runtime(t.Context(), address, handler, certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	runtime := runtimeService.(*rustHTTP3Runtime)
	serveErr := make(chan error, 1)
	go func() { serveErr <- runtime.serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = runtime.shutdown(ctx)
		select {
		case <-serveErr:
		case <-time.After(3 * time.Second):
			t.Error("Rust HTTP/3 runtime did not stop")
		}
	})

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("could not load test certificate")
	}
	transport := &http3.Transport{TLSClientConfig: &tls.Config{
		RootCAs:    roots,
		MinVersion: tls.VersionTLS13,
	}}
	t.Cleanup(func() { _ = transport.Close() })
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	baseURL := fmt.Sprintf("https://%s", address)

	for range 3 {
		response, err := client.Get(baseURL + "/ping")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent || response.StatusCode == http.StatusTooEarly {
			t.Fatalf("ping status=%d", response.StatusCode)
		}
	}

	rangeRequest, _ := http.NewRequest(http.MethodGet, baseURL+"/range", nil)
	rangeRequest.Header.Set("Range", "bytes=4-9")
	rangeResponse, err := client.Do(rangeRequest)
	if err != nil {
		t.Fatal(err)
	}
	rangeBody, _ := io.ReadAll(rangeResponse.Body)
	_ = rangeResponse.Body.Close()
	if rangeResponse.StatusCode != http.StatusPartialContent || string(rangeBody) != "456789" {
		t.Fatalf("range status=%d body=%q", rangeResponse.StatusCode, rangeBody)
	}

	payload := bytes.Repeat([]byte("audio-frame-"), 32*1024)
	echoResponse, err := client.Post(baseURL+"/echo", "application/octet-stream", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	echoBody, _ := io.ReadAll(echoResponse.Body)
	_ = echoResponse.Body.Close()
	if !bytes.Equal(echoBody, payload) {
		t.Fatalf("streamed request body length=%d, want %d", len(echoBody), len(payload))
	}

	eventsResponse, err := client.Get(baseURL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	events, _ := io.ReadAll(eventsResponse.Body)
	_ = eventsResponse.Body.Close()
	if !strings.Contains(string(events), "data: two") {
		t.Fatalf("SSE body=%q", events)
	}

	connectRequest, _ := http.NewRequest(http.MethodConnect, baseURL+"/websocket", nil)
	connectResponse, err := client.Do(connectRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, connectResponse.Body)
	_ = connectResponse.Body.Close()
	if connectResponse.StatusCode != http.StatusNotImplemented {
		t.Fatalf("CONNECT status=%d, want %d", connectResponse.StatusCode, http.StatusNotImplemented)
	}
}
