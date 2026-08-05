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

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	serverQUICHandshakeIdleTimeout = 5 * time.Second
	serverQUICKeepAlivePeriod      = 30 * time.Second
	serverHTTP3IdleTimeout         = 5 * time.Minute
	serverHTTP3AltSvcMaxAge        = 30 * 24 * time.Hour
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

	_, port, err := net.SplitHostPort(packetConn.LocalAddr().String())
	if err != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf("determining HTTP/3 listener port: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

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
			Allow0RTT:               false,
			DisablePathMTUDiscovery: false,
		},
	}

	return &http3Runtime{
		server:       server,
		packetConn:   packetConn,
		altSvcHeader: fmt.Sprintf(`h3=":%s"; ma=%d`, port, int(serverHTTP3AltSvcMaxAge/time.Second)),
	}, nil
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
