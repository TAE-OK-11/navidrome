package server

import "time"

const (
	serverQUICHandshakeIdleTimeout = 10 * time.Second
	serverQUICMaxIncomingStreams   = 256
	serverHTTP3IdleTimeout         = 15 * time.Minute
	// The inherited H2 bridge is the companion process's only data channel and
	// must outlive periods without H3 traffic. Liveness is handled by HTTP/2
	// PINGs and child-process supervision, so an application idle deadline would
	// tear down every H3 connection on an otherwise healthy server.
	serverH3BridgeIdleTimeout      = time.Duration(0)
	serverH2MaxConcurrentStreams   = 512
	serverH3BridgeMaxStreams       = 1024
	serverH2ConnectionWindow       = 4 << 20
	serverH2StreamWindow           = 512 << 10
	serverH2SendPingTimeout        = 1 * time.Minute
	serverH2PingTimeout            = 15 * time.Second
	serverH2WriteByteTimeout       = 30 * time.Second
	rustHTTP3CompressionHeader     = "X-Navidrome-H3-Compression"
)
