package server

import "testing"

func TestHTTP3TrustedProxyStreamLimitIsConservative(t *testing.T) {
	if serverQUICMaxIncomingStreams < 128 {
		t.Fatalf("MaxIncomingStreams=%d, want at least 128", serverQUICMaxIncomingStreams)
	}
	if serverQUICMaxIncomingStreams > 512 {
		t.Fatalf("MaxIncomingStreams=%d, unexpectedly high", serverQUICMaxIncomingStreams)
	}
}
