package server

import "time"

const (
	serverQUICHandshakeIdleTimeout = 10 * time.Second
	serverQUICMaxIncomingStreams   = 256
	serverHTTP3IdleTimeout         = 15 * time.Minute
)
