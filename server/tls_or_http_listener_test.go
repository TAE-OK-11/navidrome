package server

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestDemuxTLSOrHTTP_CleartextHTTP(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		_, _ = client.Write([]byte("GET /ping HTTP/1.1\r\nHost: example\r\n\r\n"))
	}()

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	conn, err := demuxTLSOrHTTP(server, cfg)
	if err != nil {
		t.Fatalf("demuxTLSOrHTTP: %v", err)
	}
	defer conn.Close()
	if _, ok := conn.(*tls.Conn); ok {
		t.Fatal("cleartext HTTP must not be wrapped in tls.Conn")
	}
	buf := make([]byte, 3)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "GET" {
		t.Fatalf("got %q, want GET", buf)
	}
}

func TestDemuxTLSOrHTTP_TLSHandshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		// Minimal TLS record header start (handshake content type 0x16).
		_, _ = client.Write([]byte{tlsHandshakeRecordType, 0x03, 0x01})
	}()

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	conn, err := demuxTLSOrHTTP(server, cfg)
	if err != nil {
		t.Fatalf("demuxTLSOrHTTP: %v", err)
	}
	defer conn.Close()
	if _, ok := conn.(*tls.Conn); !ok {
		t.Fatalf("TLS handshake byte must produce tls.Conn, got %T", conn)
	}
}

func TestTLSOrHTTPListener_ServesHTTPAndHTTPS(t *testing.T) {
	certFile := filepath.Join("testdata", "test_cert.pem")
	keyFile := filepath.Join("testdata", "test_key.pem")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dual, err := newTLSOrHTTPListener(ln, certFile, keyFile)
	if err != nil {
		t.Fatalf("newTLSOrHTTPListener: %v", err)
	}
	defer dual.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Proto", r.URL.Scheme)
		if r.TLS != nil {
			w.Header().Set("X-TLS", "1")
		} else {
			w.Header().Set("X-TLS", "0")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(dual) }()
	defer func() { _ = srv.Close() }()

	addr := "http://" + dual.Addr().String() + "/ping"
	resp, err := http.Get(addr) //nolint:gosec // loopback test server
	if err != nil {
		t.Fatalf("cleartext GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cleartext status=%d, want 204 (not Go TLS 400)", resp.StatusCode)
	}
	if got := resp.Header.Get("X-TLS"); got != "0" {
		t.Fatalf("cleartext X-TLS=%q, want 0", got)
	}

	tlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test cert
		},
	}
	httpsAddr := "https://" + dual.Addr().String() + "/ping"
	resp, err = tlsClient.Get(httpsAddr)
	if err != nil {
		t.Fatalf("https GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("https status=%d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("X-TLS"); got != "1" {
		t.Fatalf("https X-TLS=%q, want 1", got)
	}
}
