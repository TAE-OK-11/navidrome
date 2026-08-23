package server

import "testing"

func TestHTTP3RuntimePolicyBounds(t *testing.T) {
	if serverQUICHandshakeIdleTimeout <= 0 || serverQUICHandshakeIdleTimeout >= serverHTTP3IdleTimeout {
		t.Fatalf("handshake timeout=%s must be positive and below idle timeout=%s", serverQUICHandshakeIdleTimeout, serverHTTP3IdleTimeout)
	}
	if serverQUICMaxIncomingStreams < 128 || serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d outside conservative range [128,512]", serverQUICMaxIncomingStreams)
	}
	if serverH3BridgeMaxStreams < serverQUICMaxIncomingStreams {
		t.Fatalf("H3 bridge streams=%d below per-connection QUIC streams=%d", serverH3BridgeMaxStreams, serverQUICMaxIncomingStreams)
	}
	if serverH2MaxConcurrentStreams < serverQUICMaxIncomingStreams {
		t.Fatalf("public H2 streams=%d below H3 per-connection streams=%d", serverH2MaxConcurrentStreams, serverQUICMaxIncomingStreams)
	}
}
