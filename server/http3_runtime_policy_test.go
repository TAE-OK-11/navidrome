package server

import "testing"

func TestHTTP3RuntimePolicyBounds(t *testing.T) {
	if serverQUICHandshakeIdleTimeout <= 0 || serverQUICHandshakeIdleTimeout >= serverHTTP3IdleTimeout {
		t.Fatalf("handshake timeout=%s must be positive and below idle timeout=%s", serverQUICHandshakeIdleTimeout, serverHTTP3IdleTimeout)
	}
	if serverQUICMaxIncomingStreams < 128 || serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d outside conservative range [128,512]", serverQUICMaxIncomingStreams)
	}
}
