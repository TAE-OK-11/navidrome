package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/metrics"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/events"
	"github.com/navidrome/navidrome/server/publicgrpc"
	"github.com/navidrome/navidrome/ui"
	"google.golang.org/grpc"
)

type Server struct {
	router     chi.Router
	ds         model.DataStore
	appRoot    string
	broker     events.Broker
	insights   metrics.Insights
	grpcServer *grpc.Server
}

const (
	serverStartupGracePeriod = 10 * time.Millisecond
	serverShutdownTimeout    = 10 * time.Second
	serverIdleTimeout        = 5 * time.Minute
	serverMaxHeaderBytes     = 32 << 10

	serverTCPKeepAliveIdle     = 30 * time.Second
	serverTCPKeepAliveInterval = 10 * time.Second
	serverTCPKeepAliveCount    = 3
)

func New(ds model.DataStore, broker events.Broker, insights metrics.Insights) *Server {
	s := &Server{ds: ds, broker: broker, insights: insights}
	if err := initialSetup(ds); err != nil {
		log.Fatal("Initial setup failed", err)
	}
	auth.Init(s.ds)
	s.initRoutes()
	s.mountAuthenticationRoutes()
	s.mountRootRedirector()
	checkFFmpegInstallation()
	checkExternalCredentials()
	return s
}

func (s *Server) MountRouter(description, urlPath string, subRouter http.Handler) {
	urlPath = path.Join(conf.Server.BasePath, urlPath)
	log.Info(fmt.Sprintf("Mounting %s routes", description), "path", urlPath)
	s.router.Group(func(r chi.Router) {
		r.Mount(urlPath, subRouter)
	})
}

// AttachPublicGRPC multiplexes the public gRPC service onto the HTTP/2
// listener (and the HTTP/3 companion's H2 bridge). REST stays on H1/H2/H3.
func (s *Server) AttachPublicGRPC(gs *grpc.Server) {
	s.grpcServer = gs
}

// Run starts the server with the given address, and if specified, with TLS enabled.
func (s *Server) Run(ctx context.Context, addr string, port int, tlsCert string, tlsKey string) error {
	// Mount the router for the frontend assets
	s.MountRouter("WebUI", consts.URLPathUI, s.frontendAssetsHandler())

	// Determine if TLS and HTTP/3 are enabled.
	tlsEnabled := tlsCert != "" && tlsKey != ""
	http3Enabled := conf.HTTP3Enabled()
	unixSocket := strings.HasPrefix(addr, "unix:")

	if http3Enabled && !tlsEnabled {
		return errors.New("HTTP/3 requires TLSCert and TLSKey")
	}
	if http3Enabled && unixSocket {
		return errors.New("HTTP/3 is not supported with Unix sockets")
	}

	// Validate TLS certificates before starting the server.
	if tlsEnabled {
		if err := validateTLSCertificates(tlsCert, tlsKey); err != nil {
			return err
		}
	}

	// Create a listener based on the address type (either Unix socket or TCP).
	listenAddr := addr
	var listener net.Listener
	var err error
	if after, ok := strings.CutPrefix(addr, "unix:"); ok {
		socketPath := after
		listener, err = createUnixSocketFile(socketPath, conf.Server.UnixSocketPerm)
		if err != nil {
			return err
		}
	} else {
		listenAddr = fmt.Sprintf("%s:%d", addr, port)
		listener, err = createTCPListener(ctx, listenAddr)
		if err != nil {
			return fmt.Errorf("creating tcp listener: %w", err)
		}
		listenAddr = listener.Addr().String()
	}

	handler := s.publicHandler()
	server := newHTTPServer(handler, tlsEnabled, s.grpcServer != nil)
	h3 := s.startHTTP3(ctx, listenAddr, handler, tlsCert, tlsKey, server)

	errC := make(chan error, 2)
	if h3 != nil {
		go func() {
			if err := h3.serve(); err != nil {
				log.Error(ctx, "tokio-quiche HTTP/3 stopped; HTTP/1.1 and HTTP/2 remain available", err)
			}
		}()
	}
	go func() {
		var err error
		if tlsEnabled {
			log.Info("Starting server with TLS (HTTPS) enabled", "tlsCert", tlsCert, "tlsKey", tlsKey, "http3Enabled", http3Enabled, "publicGRPC", s.grpcServer != nil)
			err = server.ServeTLS(listener, tlsCert, tlsKey)
		} else {
			err = server.Serve(listener)
		}
		if !errors.Is(err, http.ErrServerClosed) {
			errC <- fmt.Errorf("serving HTTP/1.1 and HTTP/2: %w", err)
		}
	}()

	startupTime := time.Since(consts.ServerStart)

	var runErr error
	select {
	case err := <-errC:
		log.Error(ctx, "Could not start server. Aborting", err)
		runErr = fmt.Errorf("starting server: %w", err)
	case <-time.After(serverStartupGracePeriod):
		log.Info(ctx, "----> Navidrome server is ready!", "address", listenAddr, "startupTime", startupTime, "tlsEnabled", tlsEnabled, "protocols", readyProtocols(server, h3, s.grpcServer != nil))
	}

	if runErr == nil {
		select {
		case err := <-errC:
			runErr = fmt.Errorf("running server: %w", err)
		case <-ctx.Done():
		}
	}

	s.shutdownServers(ctx, server, h3)
	return runErr
}

func (s *Server) publicHandler() http.Handler {
	handler := http.Handler(s.router)
	if s.grpcServer != nil {
		log.Info("Mounting public gRPC on HTTP/2 (and HTTP/3 bridge)", "service", "navidrome.public.v1.Public")
		handler = publicgrpc.Mux(s.grpcServer, handler)
	}
	return handler
}

func (s *Server) startHTTP3(ctx context.Context, listenAddr string, handler http.Handler, tlsCert, tlsKey string, server *http.Server) http3Service {
	if !conf.HTTP3Enabled() {
		return nil
	}
	h3, err := newConfiguredHTTP3Runtime(ctx, listenAddr, handler, tlsCert, tlsKey)
	if err != nil {
		log.Warn(ctx, "tokio-quiche HTTP/3 unavailable; continuing with HTTP/1.1 and HTTP/2", err)
		server.Handler = clearHTTP3Advertisement(handler)
		return nil
	}
	server.Handler = h3.advertise(handler)
	return h3
}

func readyProtocols(server *http.Server, h3 http3Service, publicGRPC bool) string {
	protocols := server.Protocols.String()
	if h3 != nil {
		protocols += " HTTP/3(quiche)"
	}
	if publicGRPC {
		protocols += " gRPC"
	}
	return protocols
}

func (s *Server) shutdownServers(ctx context.Context, server *http.Server, h3 http3Service) {
	log.Info(ctx, "Stopping HTTP servers", "http3Enabled", h3 != nil, "publicGRPC", s.grpcServer != nil)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancel()
	server.SetKeepAlivesEnabled(false)

	shutdownErrC := make(chan error, 3)
	shutdownCount := 1
	go func() {
		shutdownErrC <- shutdownHTTPServer(shutdownCtx, server)
	}()
	if h3 != nil {
		shutdownCount++
		go func() {
			shutdownErrC <- h3.shutdown(shutdownCtx)
		}()
	}
	if s.grpcServer != nil {
		shutdownCount++
		go func() {
			stopped := make(chan struct{})
			go func() {
				s.grpcServer.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
				shutdownErrC <- nil
			case <-shutdownCtx.Done():
				s.grpcServer.Stop()
				shutdownErrC <- shutdownCtx.Err()
			}
		}()
	}

	for range shutdownCount {
		if err := <-shutdownErrC; err != nil && !errors.Is(err, context.DeadlineExceeded) {
			log.Error(ctx, "Unexpected error while shutting down HTTP server", err)
		}
	}
}

func newHTTPServer(handler http.Handler, tlsEnabled bool, publicGRPC bool) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)
	if publicGRPC && !tlsEnabled {
		// Reverse proxies that terminate TLS forward h2c; gRPC needs HTTP/2.
		protocols.SetUnencryptedHTTP2(true)
	}

	return &http.Server{
		ReadHeaderTimeout: consts.ServerReadHeaderTimeout,
		IdleTimeout:       serverIdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
		Protocols:         protocols,
		HTTP2: &http.HTTP2Config{
			MaxConcurrentStreams:          serverH2MaxConcurrentStreams,
			MaxReceiveBufferPerConnection: serverH2ConnectionWindow,
			MaxReceiveBufferPerStream:     serverH2StreamWindow,
			SendPingTimeout:               serverH2SendPingTimeout,
			PingTimeout:                   serverH2PingTimeout,
			WriteByteTimeout:              serverH2WriteByteTimeout,
		},
		Handler: handler,
	}
}

func createTCPListener(ctx context.Context, address string) (net.Listener, error) {
	config := net.ListenConfig{
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     serverTCPKeepAliveIdle,
			Interval: serverTCPKeepAliveInterval,
			Count:    serverTCPKeepAliveCount,
		},
	}
	listener, err := config.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("creating tcp listener: %w", err)
	}
	return listener, nil
}

func createUnixSocketFile(socketPath string, socketPerm string) (net.Listener, error) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing previous unix socket file: %w", err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("creating unix socket listener: %w", err)
	}
	perm, err := strconv.ParseUint(socketPerm, 8, 32)
	if err != nil {
		return nil, fmt.Errorf("parsing unix socket file permissions: %w", err)
	}
	err = os.Chmod(socketPath, os.FileMode(perm))
	if err != nil {
		return nil, fmt.Errorf("updating permission of unix socket file: %w", err)
	}
	return listener, nil
}

func (s *Server) initRoutes() {
	s.appRoot = path.Join(conf.Server.BasePath, consts.URLPathUI)

	r := chi.NewRouter()

	// Inbound HTTP pipeline shared by every mounted API:
	// security/CORS → request id/IP → recover → compression → JWT → request log.
	defaultMiddlewares := chi.Middlewares{
		secureMiddleware(),
		corsHandler(),
		middleware.RequestID,
		realIPMiddleware,
		middleware.Recoverer,
		middleware.Heartbeat("/ping"),
		robotsTXT(ui.BuildAssets()),
		serverAddressMiddleware,
		clientUniqueIDMiddleware,
		compressMiddleware(),
		loggerInjector,
		JWTVerifier,
	}

	if conf.Server.DevActivityPanel {
		r.Group(func(r chi.Router) {
			r.Use(defaultMiddlewares...)
			r.Use(Authenticator(s.ds))
			r.Use(JWTRefresher)
			r.Handle(path.Join(conf.Server.BasePath, consts.URLPathNativeAPI, "events"), s.broker)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(defaultMiddlewares...)
		r.Use(requestLogger)
		s.router = r
	})
}

func (s *Server) mountAuthenticationRoutes() chi.Router {
	r := s.router
	return r.Route(path.Join(conf.Server.BasePath, "/auth"), func(r chi.Router) {
		if conf.Server.AuthRequestLimit > 0 {
			log.Info("Login rate limit set", "requestLimit", conf.Server.AuthRequestLimit,
				"windowLength", conf.Server.AuthWindowLength)

			rateLimiter := httprate.LimitByIP(conf.Server.AuthRequestLimit, conf.Server.AuthWindowLength)
			r.With(rateLimiter).Post("/login", login(s.ds))
		} else {
			log.Warn("Login rate limit is disabled! Consider enabling it to be protected against brute-force attacks")
			r.Post("/login", login(s.ds))
		}
		r.Post("/createAdmin", createAdmin(s.ds))
	})
}

func (s *Server) mountRootRedirector() {
	r := s.router
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.appRoot+"/", http.StatusFound)
	})
	r.Get(s.appRoot, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, s.appRoot+"/", http.StatusFound)
	})
}

func (s *Server) frontendAssetsHandler() http.Handler {
	r := chi.NewRouter()

	r.Handle("/", Index(s.ds, ui.BuildAssets()))
	r.Handle("/*", http.StripPrefix(s.appRoot, PrecompressedFileServer(ui.BuildAssets())))
	return r
}

func validateTLSCertificates(certFile, keyFile string) error {
	keyData, err := os.ReadFile(keyFile) //nolint:gosec
	if err != nil {
		return fmt.Errorf("reading TLS key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return errors.New("TLS key file does not contain a valid PEM block")
	}

	if isEncryptedPEM(block, keyData) {
		return errors.New("TLS private key is encrypted (password-protected). " +
			"Navidrome does not support encrypted private keys. " +
			"Please decrypt your key using: openssl pkey -in <encrypted-key> -out <decrypted-key>")
	}

	_, err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("loading TLS certificate/key pair: %w", err)
	}

	return nil
}

func isEncryptedPEM(block *pem.Block, rawData []byte) bool {
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		return true
	}

	if block.Headers != nil {
		if procType, ok := block.Headers["Proc-Type"]; ok && strings.Contains(procType, "ENCRYPTED") {
			return true
		}
	}

	if bytes.Contains(rawData, []byte("DEK-Info:")) || bytes.Contains(rawData, []byte("Proc-Type: 4,ENCRYPTED")) {
		return true
	}

	return false
}
