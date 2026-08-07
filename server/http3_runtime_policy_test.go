package server

import (
	"testing"
	"time"
)

func TestHTTP3RuntimePolicyBounds(t *testing.T) {
	if serverQUICKeepAlivePeriod <= 0 || serverQUICKeepAlivePeriod >= serverHTTP3IdleTimeout {
		t.Fatalf("keepalive=%s must be positive and below idle timeout=%s", serverQUICKeepAlivePeriod, serverHTTP3IdleTimeout)
	}
	if serverQUICHandshakeIdleTimeout <= 0 || serverQUICHandshakeIdleTimeout >= serverHTTP3IdleTimeout {
		t.Fatalf("handshake timeout=%s must be positive and below idle timeout=%s", serverQUICHandshakeIdleTimeout, serverHTTP3IdleTimeout)
	}
	if serverQUICMaxIncomingStreams < 128 || serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d outside conservative trusted-proxy range [128,512]", serverQUICMaxIncomingStreams)
	}
	if serverHTTP3AltSvcMaxAge <= 0 || serverHTTP3AltSvcMaxAge > 24*time.Hour {
		t.Fatalf("Alt-Svc max-age=%s must stay within one day for fast rollback", serverHTTP3AltSvcMaxAge)
	}
}
