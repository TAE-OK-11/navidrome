package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	serverQUICHandshakeIdleTimeout = 10 * time.Second
	serverQUICKeepAlivePeriod      = 20 * time.Second
	serverQUICMaxIncomingStreams   = 256
	serverHTTP3IdleTimeout         = 15 * time.Minute
	serverHTTP3AltSvcMaxAge        = 24 * time.Hour
	serverQUICSocketBufferSize     = 7 * 1024 * 1024
	serverQUICAllow0RTT            = false
)

type http3Runtime struct {
	server       *http3.Server
	packetConn   net.PacketConn
	altSvcHeader string

	shutdownOnce sync.Once
	shutdownErr  error
}

func newHTTP3Runtime(ctx context.Context, addr string, handler http.Handler, certFile, keyFile string) (*http3Runtime, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading HTTP/3 TLS certificate/key pair: %w", err)
	}

	packetConn, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp", addr)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP/3 UDP listener: %w", err)
	}
	tuneQUICSocketBuffers(ctx, packetConn)

	_, port, err := net.SplitHostPort(packetConn.LocalAddr().String())
	if err != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf("determining HTTP/3 listener port: %w", err)
	}

	// ConfigureTLSConfig installs the HTTP/3 ALPN handling required by quic-go.
	// Keep TLS 1.3 as the floor; session tickets remain enabled for fast resumed
	// handshakes even though HTTP request data itself is not accepted in 0-RTT.
	tlsConfig := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})

	server := &http3.Server{
		Addr:           packetConn.LocalAddr().String(),
		TLSConfig:      tlsConfig,
		Handler:        handler,
		MaxHeaderBytes: serverMaxHeaderBytes,
		IdleTimeout:    serverHTTP3IdleTimeout,
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout:    serverQUICHandshakeIdleTimeout,
			MaxIdleTimeout:          serverHTTP3IdleTimeout,
			KeepAlivePeriod:         serverQUICKeepAlivePeriod,
			MaxIncomingStreams:      serverQUICMaxIncomingStreams,
			// Do not accept HTTP requests in QUIC early data. A previous route-level
			// replay guard returned HTTP 425 for requests that arrived before the TLS
			// handshake completed, leaking transport details into apps and forcing an
			// extra HTTP retry. Rejecting 0-RTT at the QUIC/TLS layer keeps the retry
			// transparent to conforming client stacks while established HTTP/3
			// connections keep their normal performance characteristics.
			Allow0RTT:               serverQUICAllow0RTT,
			DisablePathMTUDiscovery: false,
			EnableDatagrams:          false,
		},
	}

	return &http3Runtime{
		server:       server,
		packetConn:   packetConn,
		altSvcHeader: fmt.Sprintf(`h3=":%s"; ma=%d`, port, int(serverHTTP3AltSvcMaxAge/time.Second)),
	}, nil
}

func tuneQUICSocketBuffers(ctx context.Context, packetConn net.PacketConn) {
	udpConn, ok := packetConn.(*net.UDPConn)
	if !ok {
		log.Warn(ctx, "HTTP/3 packet connection is not a native UDP socket; QUIC kernel optimizations may be unavailable")
		return
	}
	if err := udpConn.SetReadBuffer(serverQUICSocketBufferSize); err != nil {
		log.Warn(ctx, "Could not raise HTTP/3 UDP receive buffer", "size", serverQUICSocketBufferSize, err)
	}
	if err := udpConn.SetWriteBuffer(serverQUICSocketBufferSize); err != nil {
		log.Warn(ctx, "Could not raise HTTP/3 UDP send buffer", "size", serverQUICSocketBufferSize, err)
	}
}

func (r *http3Runtime) serve() error {
	return r.server.Serve(r.packetConn)
}

func (r *http3Runtime) advertise(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Alt-Svc", r.altSvcHeader)
		next.ServeHTTP(w, req)
	})
}

func (r *http3Runtime) shutdown(ctx context.Context) error {
	r.shutdownOnce.Do(func() {
		gracefulErr := r.server.Shutdown(ctx)
		var forceErr error
		if gracefulErr != nil {
			forceErr = r.server.Close()
		}
		closeErr := r.packetConn.Close()

		r.shutdownErr = errors.Join(
			normalizeHTTP3CloseError(gracefulErr),
			normalizeHTTP3CloseError(forceErr),
			normalizeHTTP3CloseError(closeErr),
		)
	})
	return r.shutdownErr
}

func normalizeHTTP3CloseError(err error) error {
	if isExpectedHTTP3ServerClose(err) {
		return nil
	}
	return err
}

func isExpectedHTTP3ServerClose(err error) bool {
	return err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) || errors.Is(err, quic.ErrServerClosed)
}
