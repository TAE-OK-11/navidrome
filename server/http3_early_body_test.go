package server

import "testing"

func TestHTTP3ZeroRTTDisabledToAvoidAppVisible425(t *testing.T) {
	if serverQUICAllow0RTT {
		t.Fatal("HTTP/3 0-RTT must stay disabled so early requests are handled by the QUIC/TLS handshake instead of surfacing HTTP 425")
	}
}

func TestHTTP3TrustedProxyStreamLimitIsRaisedConservatively(t *testing.T) {
	if serverQUICMaxIncomingStreams <= 100 {
		t.Fatalf("MaxIncomingStreams=%d, want above quic-go default 100", serverQUICMaxIncomingStreams)
	}
	if serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d, unexpectedly high", serverQUICMaxIncomingStreams)
	}
}
