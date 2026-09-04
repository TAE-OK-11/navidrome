package integration

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestArtworkDestinationIPScope(t *testing.T) {
	for _, raw := range []string{
		"0.0.0.0", "127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254",
		"192.0.2.1", "198.18.0.1", "224.0.0.1", "240.0.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1",
	} {
		if isSafeArtworkIP(net.ParseIP(raw)) {
			t.Errorf("expected special-use address %s to be rejected", raw)
		}
	}
	if !isSafeArtworkIP(net.ParseIP("8.8.8.8")) || !isSafeArtworkIP(net.ParseIP("2606:4700:4700::1111")) {
		t.Fatal("expected representative public unicast addresses to remain allowed")
	}
}

func TestSSRFDialerBlocksResolvedPrivateAddressAndRedirect(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		_, _ = w.Write([]byte("private"))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://redirected.test/secret", http.StatusFound)
	}))
	defer redirect.Close()

	dialer := ssrfDialer{
		lookup: func(_ context.Context, _, host string) ([]net.IP, error) {
			switch host {
			case "initial.test":
				return []net.IP{net.ParseIP("8.8.8.8")}, nil
			case "redirected.test":
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			default:
				return nil, errors.New("unexpected host")
			}
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			if strings.HasPrefix(address, "8.8.8.8:") {
				return d.DialContext(ctx, network, redirect.Listener.Addr().String())
			}
			return d.DialContext(ctx, network, target.Listener.Addr().String())
		},
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
	resp, err := client.Get("http://initial.test/start")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "disallowed address") {
		t.Fatalf("expected redirected private address to be blocked, got %v", err)
	}
	if targetHits.Load() != 0 {
		t.Fatal("private redirect target was reached")
	}
}

func TestArtworkHTTPClientBlocksLoopbackFallback(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	g := &Gateway{artworkFallback: newSSRFTransport()}
	client := g.ArtworkHTTPClient(5 * time.Second)
	resp, err := client.Get(server.URL)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || hits.Load() != 0 {
		t.Fatalf("expected loopback artwork fetch to be blocked, err=%v hits=%d", err, hits.Load())
	}
}
