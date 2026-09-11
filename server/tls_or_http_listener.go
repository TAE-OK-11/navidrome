package server

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
)

const (
	// tlsHandshakeRecordType is the first byte of a TLS handshake record
	// (RFC 8446 §5.1). Cleartext HTTP methods start with ASCII letters instead.
	tlsHandshakeRecordType byte = 0x16
	tlsOrHTTPPeekSize           = 1
)

// tlsOrHTTPListener accepts both TLS and cleartext HTTP on one TCP port.
// Connections whose first byte is a TLS handshake (0x16) are wrapped with
// tls.Server; every other connection is passed through for plain HTTP/1.1 or
// h2c. This avoids Go's default "400 Client sent an HTTP request to an HTTPS
// server" when Tailscale/WireGuard clients call http://host:4533 while TLS
// certificates are also configured (e.g. for HTTPS or HTTP/3).
type tlsOrHTTPListener struct {
	net.Listener
	config *tls.Config
}

func newTLSOrHTTPListener(inner net.Listener, certFile, keyFile string) (net.Listener, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS certificate for dual HTTP/HTTPS listener: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2", "http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}
	return &tlsOrHTTPListener{Listener: inner, config: cfg}, nil
}

func (l *tlsOrHTTPListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return demuxTLSOrHTTP(conn, l.config)
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func demuxTLSOrHTTP(conn net.Conn, cfg *tls.Config) (net.Conn, error) {
	br := bufio.NewReaderSize(conn, 4096)
	b, err := br.Peek(tlsOrHTTPPeekSize)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	wrapped := &bufferedConn{Conn: conn, r: br}
	if b[0] == tlsHandshakeRecordType {
		return tls.Server(wrapped, cfg), nil
	}
	return wrapped, nil
}
