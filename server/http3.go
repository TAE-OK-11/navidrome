package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

const (
	serverQUICHandshakeIdleTimeout = 5 * time.Second
	serverQUICKeepAlivePeriod      = 30 * time.Second
	serverQUICMaxIncomingStreams   = 256
	serverHTTP3IdleTimeout         = 5 * time.Minute
	serverHTTP3AltSvcMaxAge        = 24 * time.Hour
)

var safeSubsonic0RTTEndpoints = map[string]struct{}{
	"getopensubsonicextensions": {},
	"ping":                      {},
	"getlicense":                {},
	"getmusicfolders":           {},
	"getgenres":                 {},
	"getscanstatus":             {},
	"getindexes":                {},
	"getartists":                {},
	"getmusicdirectory":         {},
	"getartist":                 {},
	"getalbum":                  {},
	"getsong":                   {},
	"getalbumlist":              {},
	"getalbumlist2":             {},
	"getstarred":                {},
	"getstarred2":               {},
	"getnowplaying":             {},
	"getrandomsongs":            {},
	"getsongsbygenre":           {},
	"getplaylists":              {},
	"getplaylist":               {},
	"getbookmarks":              {},
	"getplayqueue":              {},
	"getplayqueuebyindex":       {},
	"search2":                   {},
	"search3":                   {},
	"getuser":                   {},
	"getusers":                  {},
	"getinternetradiostations":  {},
	"getshares":                 {},
	// These are high-frequency, idempotent reads used while rendering a
	// freshly opened screen. Letting them complete in 0-RTT avoids a 425 +
	// retry round-trip on every resumed QUIC connection. Streaming remains
	// intentionally excluded because replaying it can duplicate large bodies.
	"getcoverart":      {},
	"getlyrics":        {},
	"getlyricsbysongid": {},
}

var safeHTTP3StaticExtensions = map[string]struct{}{
	".avif":  {},
	".css":   {},
	".gif":   {},
	".html":  {},
	".ico":   {},
	".jpeg":  {},
	".jpg":   {},
	".js":    {},
	".json":  {},
	".png":   {},
	".svg":   {},
	".webp":  {},
	".woff":  {},
	".woff2": {},
}

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

	// ConfigureTLSConfig installs the HTTP/3 ALPN handling required by quic-go.
	// Keep TLS 1.3 as the floor; session tickets remain enabled for 0-RTT.
	tlsConfig := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})

	server := &http3.Server{
		Addr:           packetConn.LocalAddr().String(),
		TLSConfig:      tlsConfig,
		Handler:        guardHTTP3EarlyData(handler),
		MaxHeaderBytes: serverMaxHeaderBytes,
		IdleTimeout:    serverHTTP3IdleTimeout,
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout:    serverQUICHandshakeIdleTimeout,
			MaxIdleTimeout:          serverHTTP3IdleTimeout,
			KeepAlivePeriod:         serverQUICKeepAlivePeriod,
			MaxIncomingStreams:      serverQUICMaxIncomingStreams,
			Allow0RTT:               conf.HTTP3Allow0RTT(),
			DisablePathMTUDiscovery: false,
		},
	}

	return &http3Runtime{
		server:       server,
		packetConn:   packetConn,
		altSvcHeader: fmt.Sprintf(`h3=\":%s\"; ma=%d`, port, int(serverHTTP3AltSvcMaxAge/time.Second)),
	}, nil
}

func guardHTTP3EarlyData(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isPotentialHTTP3Replay(req) && !isSafeHTTP3EarlyRequest(req) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "0")
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusTooEarly)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func isPotentialHTTP3Replay(req *http.Request) bool {
	return req.TLS != nil && !req.TLS.HandshakeComplete
}

func isSafeHTTP3EarlyRequest(req *http.Request) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	// Early-data reads never need a request body in Navidrome. Reject bodies,
	// including unknown-length bodies, to keep replayable payloads minimal.
	if req.ContentLength != 0 {
		return false
	}

	requestPath := http3PathWithoutBasePath(req.URL.Path)
	if requestPath == "/" || requestPath == "/ping" || isSafeHTTP3StaticPath(requestPath) {
		return true
	}

	// Native API follows HTTP method semantics, but OAuth / scrobbler auth
	// callbacks mounted below /api can mutate stored credentials even on GET.
	// Keep those callbacks out of replayable early data.
	if strings.HasPrefix(requestPath, "/api/") {
		return !hasPathPrefix(requestPath, "/api/lastfm") &&
			!hasPathPrefix(requestPath, "/api/listenbrainz")
	}

	if !strings.HasPrefix(requestPath, "/rest/") {
		return false
	}

	endpoint := strings.TrimPrefix(requestPath, "/rest/")
	if slash := strings.IndexByte(endpoint, '/'); slash >= 0 {
		endpoint = endpoint[:slash]
	}
	endpoint = strings.TrimSuffix(endpoint, ".view")
	endpoint = strings.ToLower(endpoint)

	// Subsonic permits state-changing operations over GET. Keep a positive
	// allowlist instead of broad get*/search* matching. Small, idempotent UI
	// reads may use 0-RTT, while audio streaming, transcoding and other large or
	// state-changing operations wait for the handshake to avoid replay cost.
	_, ok := safeSubsonic0RTTEndpoints[endpoint]
	return ok
}

func isSafeHTTP3StaticPath(requestPath string) bool {
	if requestPath == "" || strings.HasSuffix(requestPath, "/") {
		return false
	}
	_, ok := safeHTTP3StaticExtensions[strings.ToLower(filepath.Ext(requestPath))]
	return ok
}

func http3PathWithoutBasePath(requestPath string) string {
	basePath := strings.TrimSuffix(conf.Server.BasePath, "/")
	if basePath == "" || basePath == "/" {
		return requestPath
	}
	if requestPath == basePath {
		return "/"
	}
	if strings.HasPrefix(requestPath, basePath+"/") {
		return strings.TrimPrefix(requestPath, basePath)
	}
	return requestPath
}

func hasPathPrefix(requestPath, prefix string) bool {
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
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
