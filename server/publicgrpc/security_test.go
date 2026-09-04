package publicgrpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/navidrome/navidrome/conf"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestCheckClientNetworkAllowlist(t *testing.T) {
	conf.SetPublicGRPCNetworkPolicyForTest("10.77.0.0/24", "")
	t.Cleanup(func() { conf.SetPublicGRPCNetworkPolicyForTest("", "") })

	ctx := peerContext("10.77.0.5:1234")
	if err := checkClientNetwork(ctx); err != nil {
		t.Fatalf("allowed client rejected: %v", err)
	}

	ctx = peerContext("203.0.113.1:1234")
	err := checkClientNetwork(ctx)
	if err == nil {
		t.Fatal("disallowed client must be rejected")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestClientIPTrustedProxyForwardedFor(t *testing.T) {
	conf.SetPublicGRPCNetworkPolicyForTest("", "127.0.0.1/32")
	t.Cleanup(func() { conf.SetPublicGRPCNetworkPolicyForTest("", "") })

	ctx := metadata.NewIncomingContext(
		peerContext("127.0.0.1:9000"),
		metadata.Pairs("x-forwarded-for", "10.77.0.8, 203.0.113.2"),
	)
	if got := clientIP(ctx); got != "10.77.0.8" {
		t.Fatalf("clientIP=%q", got)
	}
}

func TestLoginRateLimitUnknownPeerFailClosed(t *testing.T) {
	conf.SetPublicGRPCEnabledForTest(true)
	oldLimit := conf.Server.AuthRequestLimit
	oldWindow := conf.Server.AuthWindowLength
	conf.Server.AuthRequestLimit = 2
	conf.Server.AuthWindowLength = time.Minute
	t.Cleanup(func() {
		conf.Server.AuthRequestLimit = oldLimit
		conf.Server.AuthWindowLength = oldWindow
	})

	ctx := context.Background()
	for range 2 {
		if err := checkLoginRateLimit(ctx); err != nil {
			t.Fatalf("expected allow: %v", err)
		}
	}
	err := checkLoginRateLimit(ctx)
	if err == nil {
		t.Fatal("unknown peer must still be rate limited")
	}
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func peerContext(addr string) context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: parseAddr(addr)})
}

func parseAddr(addr string) net.Addr {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return &net.TCPAddr{IP: net.ParseIP(addr)}
	}
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return &net.TCPAddr{IP: net.ParseIP(host), Port: p}
}
