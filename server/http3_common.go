package server

import "time"

const (
	serverQUICHandshakeIdleTimeout = 10 * time.Second
	serverQUICMaxIncomingStreams   = 256
	serverHTTP3IdleTimeout         = 15 * time.Minute
	serverH2MaxConcurrentStreams   = 512
	serverH3BridgeMaxStreams       = 1024
	serverH2ConnectionWindow       = 4 << 20
	serverH2StreamWindow           = 512 << 10
	serverH2SendPingTimeout        = 1 * time.Minute
	serverH2PingTimeout            = 15 * time.Second
	serverH2WriteByteTimeout       = 30 * time.Second
)
