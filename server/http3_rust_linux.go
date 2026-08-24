//go:build linux

package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
	"golang.org/x/sys/unix"
)

const (
	rustHTTP3ControlFD          = 3
	rustHTTP3BridgeFD           = 4
	rustHTTP3TokenHeader        = "X-Navidrome-H3-Token" //nolint:gosec // header name only; token value is generated at runtime
	rustHTTP3AuthorityHeader    = "X-Navidrome-H3-Authority"
	rustHTTP3RemoteAddrHeader   = "X-Navidrome-H3-Remote-Addr"
	rustHTTP3StartupTimeout     = 15 * time.Second
	rustHTTP3RestartMinDelay    = 100 * time.Millisecond
	rustHTTP3RestartMaxDelay    = 5 * time.Second
	rustHTTP3ControlMaxLineSize = 64 * 1024
	rustHTTP3BridgeSocketBuffer = 4 << 20
)

type rustHTTP3Config struct {
	UDPAddress           string  `json:"udp_address"`
	Certificate          string  `json:"certificate"`
	PrivateKey           string  `json:"private_key"`
	InternalToken        string  `json:"internal_token"`
	AltSvcMaxAgeSeconds  int64   `json:"alt_svc_max_age_seconds"`
	QlogDir              string  `json:"qlog_dir,omitempty"`
	HandshakeTimeoutSecs uint64  `json:"handshake_timeout_seconds"`
	IdleTimeoutSecs      uint64  `json:"idle_timeout_seconds"`
	MaxConcurrentStreams uint64  `json:"max_concurrent_streams"`
	MaxConnections       int     `json:"max_connections"`
	MaxConnectionsPerIP  int     `json:"max_connections_per_ip"`
	MaxInFlightRequests  int     `json:"max_in_flight_requests"`
	ConnectionRate       float64 `json:"connection_rate_per_second"`
	ConnectionBurst      int     `json:"connection_burst"`
	CongestionControl    string  `json:"congestion_control"`
}

type rustHTTP3Runtime struct {
	ctx        context.Context
	cancel     context.CancelFunc
	config     rustHTTP3Config
	binaryPath string
	altSvc     string

	internalServer *http.Server
	bridgeListener *inheritedConnListener

	ready    atomic.Bool
	stopping atomic.Bool
	mu       sync.Mutex
	cmd      *exec.Cmd
	control  net.Conn
}

func newRustHTTP3Runtime(
	ctx context.Context,
	addr string,
	handler http.Handler,
	certFile, keyFile string,
) (http3Service, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generating HTTP/3 bridge token: %w", err)
	}
	if conf.HTTP3MaxConnections() < 1 || conf.HTTP3MaxConnectionsPerIP() < 1 ||
		conf.HTTP3MaxInFlightRequests() < 1 ||
		conf.HTTP3ConnectionRatePerSecond() <= 0 || conf.HTTP3ConnectionBurst() < 0 {
		return nil, errors.New("HTTP/3 admission limits must be positive (connection burst may be zero)")
	}
	congestionControl := conf.HTTP3CongestionControl()
	if !conf.ValidHTTP3CongestionControl(congestionControl) {
		return nil, fmt.Errorf("unsupported HTTP/3 congestion control %q", congestionControl)
	}
	token := hex.EncodeToString(tokenBytes)

	listener := newInheritedConnListener()
	protocols := new(http.Protocols)
	protocols.SetHTTP1(false)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	internalServer := &http.Server{
		ReadHeaderTimeout: consts.ServerReadHeaderTimeout,
		IdleTimeout:       serverHTTP3IdleTimeout,
		MaxHeaderBytes:    serverMaxHeaderBytes,
		Protocols:         protocols,
		HTTP2: &http.HTTP2Config{
			MaxConcurrentStreams:          serverH3BridgeMaxStreams,
			MaxReceiveBufferPerConnection: serverH2ConnectionWindow,
			MaxReceiveBufferPerStream:     serverH2StreamWindow,
			SendPingTimeout:               serverH2SendPingTimeout,
			PingTimeout:                   serverH2PingTimeout,
			WriteByteTimeout:              serverH2WriteByteTimeout,
		},
		Handler: authenticatedHTTP3Bridge(token, handler),
	}

	runtimeCtx, cancel := context.WithCancel(ctx)
	r := &rustHTTP3Runtime{
		ctx:            runtimeCtx,
		cancel:         cancel,
		internalServer: internalServer,
		bridgeListener: listener,
		binaryPath:     resolveHTTP3GatewayPath(),
		altSvc:         altSvcForAddress(addr, conf.HTTP3AltSvcMaxAge()),
		config: rustHTTP3Config{
			UDPAddress:           addr,
			Certificate:          certFile,
			PrivateKey:           keyFile,
			InternalToken:        token,
			AltSvcMaxAgeSeconds:  int64(conf.HTTP3AltSvcMaxAge() / time.Second),
			QlogDir:              conf.HTTP3QlogDir(),
			HandshakeTimeoutSecs: uint64(serverQUICHandshakeIdleTimeout / time.Second),
			IdleTimeoutSecs:      uint64(serverHTTP3IdleTimeout / time.Second),
			MaxConcurrentStreams: serverQUICMaxIncomingStreams,
			MaxConnections:       conf.HTTP3MaxConnections(),
			MaxConnectionsPerIP:  conf.HTTP3MaxConnectionsPerIP(),
			MaxInFlightRequests:  conf.HTTP3MaxInFlightRequests(),
			ConnectionRate:       conf.HTTP3ConnectionRatePerSecond(),
			ConnectionBurst:      conf.HTTP3ConnectionBurst(),
			CongestionControl:    congestionControl,
		},
	}

	go func() {
		if err := internalServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(runtimeCtx, "Private inherited HTTP/3 bridge stopped", err)
		}
	}()

	if err := r.startChild(); err != nil {
		cancel()
		_ = internalServer.Close()
		_ = listener.Close()
		return nil, err
	}
	return r, nil
}

func resolveHTTP3GatewayPath() string {
	if configured := strings.TrimSpace(conf.HTTP3GatewayPath()); configured != "" {
		return configured
	}
	executable, err := os.Executable()
	if err != nil {
		return "navidrome-h3"
	}
	return filepath.Join(filepath.Dir(executable), "navidrome-h3")
}

func altSvcForAddress(addr string, maxAge time.Duration) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(`h3=":%s"; ma=%d`, port, max(0, int(maxAge/time.Second)))
}

func authenticatedHTTP3Bridge(token string, next http.Handler) http.Handler {
	tokenBytes := []byte(token)
	tlsState := &tls.ConnectionState{
		Version:           tls.VersionTLS13,
		HandshakeComplete: true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// The bridge connection is an inherited AF_UNIX socketpair and therefore
		// has no externally reachable address. Keep the per-process token as
		// defense in depth and compare it in constant time.
		provided := req.Header.Get(rustHTTP3TokenHeader)
		if subtle.ConstantTimeCompare([]byte(provided), tokenBytes) != 1 {
			http3BridgeRejected.WithLabelValues("invalid_token").Inc()
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		req.Header.Del(rustHTTP3TokenHeader)
		if remoteAddr := req.Header.Get(rustHTTP3RemoteAddrHeader); remoteAddr != "" {
			if _, remoteErr := netip.ParseAddrPort(remoteAddr); remoteErr != nil {
				http3BridgeRejected.WithLabelValues("invalid_remote_addr").Inc()
				http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			req.RemoteAddr = remoteAddr
		}
		req.Header.Del(rustHTTP3RemoteAddrHeader)
		if authority := req.Header.Get(rustHTTP3AuthorityHeader); authority != "" {
			req.Host = authority
		}
		req.Header.Del(rustHTTP3AuthorityHeader)
		// The outer quiche transport terminated TLS 1.3. Preserve HTTPS semantics
		// for existing middleware without exposing the private HTTP/2 hop.
		req.Proto = "HTTP/3.0"
		req.ProtoMajor = 3
		req.ProtoMinor = 0
		req.TLS = tlsState
		next.ServeHTTP(w, req)
	})
}

func inheritedSocketpair(parentName, childName string, socketBuffer int) (net.Conn, *os.File, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	if socketBuffer > 0 {
		for _, fd := range fds {
			if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, socketBuffer); err != nil {
				_ = unix.Close(fds[0])
				_ = unix.Close(fds[1])
				return nil, nil, fmt.Errorf("setting inherited socket receive buffer: %w", err)
			}
			if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, socketBuffer); err != nil {
				_ = unix.Close(fds[0])
				_ = unix.Close(fds[1])
				return nil, nil, fmt.Errorf("setting inherited socket send buffer: %w", err)
			}
		}
	}
	parentFile := os.NewFile(uintptr(fds[0]), parentName)
	child := os.NewFile(uintptr(fds[1]), childName)
	if parentFile == nil || child == nil {
		if parentFile != nil {
			_ = parentFile.Close()
		}
		if child != nil {
			_ = child.Close()
		}
		return nil, nil, errors.New("creating inherited socketpair files")
	}
	parent, err := net.FileConn(parentFile)
	_ = parentFile.Close()
	if err != nil {
		_ = child.Close()
		return nil, nil, err
	}
	return parent, child, nil
}

func (r *rustHTTP3Runtime) startChild() error {
	parent, child, err := inheritedSocketpair("navidrome-h3-control-parent", "navidrome-h3-control-child", 0)
	if err != nil {
		return fmt.Errorf("creating HTTP/3 control socket: %w", err)
	}
	bridgeParent, bridgeChild, err := inheritedSocketpair(
		"navidrome-h3-bridge-parent", "navidrome-h3-bridge-child", rustHTTP3BridgeSocketBuffer,
	)
	if err != nil {
		_ = parent.Close()
		_ = child.Close()
		return fmt.Errorf("creating HTTP/3 data bridge socket: %w", err)
	}

	cmd := exec.CommandContext(r.ctx, r.binaryPath) //nolint:gosec // administrator-configured companion path
	cmd.ExtraFiles = []*os.File{child, bridgeChild}
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("NAVIDROME_H3_CONTROL_FD=%d", rustHTTP3ControlFD),
		fmt.Sprintf("NAVIDROME_H3_BRIDGE_FD=%d", rustHTTP3BridgeFD),
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = parent.Close()
		_ = child.Close()
		_ = bridgeParent.Close()
		_ = bridgeChild.Close()
		return fmt.Errorf("starting tokio-quiche HTTP/3 companion %q: %w", r.binaryPath, err)
	}
	_ = child.Close()
	_ = bridgeChild.Close()

	failed := true
	bridgeHandedOff := false
	defer func() {
		if failed {
			_ = parent.Close()
			if !bridgeHandedOff {
				_ = bridgeParent.Close()
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	if err := r.bridgeListener.add(bridgeParent); err != nil {
		return fmt.Errorf("registering inherited HTTP/3 bridge: %w", err)
	}
	bridgeHandedOff = true

	if err := json.NewEncoder(parent).Encode(r.config); err != nil { //nolint:gosec // sent only over the inherited private socketpair
		return fmt.Errorf("sending HTTP/3 companion configuration: %w", err)
	}
	if err := parent.SetReadDeadline(time.Now().Add(rustHTTP3StartupTimeout)); err != nil {
		return fmt.Errorf("setting HTTP/3 readiness deadline: %w", err)
	}
	reader := bufio.NewReaderSize(parent, rustHTTP3ControlMaxLineSize)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			waitErr := cmd.Wait()
			failed = false
			_ = parent.Close()
			if waitErr != nil {
				return fmt.Errorf("waiting for HTTP/3 companion readiness: %w", errors.Join(err, waitErr))
			}
			return fmt.Errorf("waiting for HTTP/3 companion readiness: %w", err)
		}
		return fmt.Errorf("waiting for HTTP/3 companion readiness: %w", err)
	}
	if strings.TrimSpace(line) != "READY" {
		return fmt.Errorf("HTTP/3 companion returned unexpected readiness message %q", strings.TrimSpace(line))
	}
	_ = parent.SetReadDeadline(time.Time{})

	r.mu.Lock()
	r.cmd = cmd
	r.control = parent
	r.mu.Unlock()
	r.ready.Store(true)
	http3CompanionUp.Set(1)
	failed = false
	log.Info(r.ctx, "Tokio-quiche HTTP/3 companion is ready", "udpAddress", r.config.UDPAddress,
		"bridge", "inherited-af_unix+h2", "binary", r.binaryPath,
		"congestionControl", r.config.CongestionControl)
	return nil
}

func (r *rustHTTP3Runtime) serve() error {
	delay := rustHTTP3RestartMinDelay
	for {
		r.mu.Lock()
		cmd := r.cmd
		r.mu.Unlock()
		if cmd == nil {
			return nil
		}

		err := cmd.Wait()
		r.ready.Store(false)
		http3CompanionUp.Set(0)
		r.mu.Lock()
		if r.control != nil {
			_ = r.control.Close()
		}
		r.control = nil
		r.cmd = nil
		r.mu.Unlock()
		if r.stopping.Load() || r.ctx.Err() != nil {
			return nil //nolint:nilerr // child exit is expected during an explicit or contextual shutdown
		}

		log.Error(r.ctx, "Tokio-quiche HTTP/3 companion stopped; H1/H2 remain available", err)
		http3CompanionRestarts.Inc()
		for !r.stopping.Load() && r.ctx.Err() == nil {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-r.ctx.Done():
				timer.Stop()
				return nil
			}
			if err := r.startChild(); err == nil {
				delay = rustHTTP3RestartMinDelay
				break
			} else {
				log.Warn(r.ctx, "Could not restart tokio-quiche HTTP/3 companion", err)
				delay = min(delay*2, rustHTTP3RestartMaxDelay)
			}
		}
	}
}

func (r *rustHTTP3Runtime) advertise(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.ready.Load() && r.altSvc != "" {
			w.Header().Set("Alt-Svc", r.altSvc)
		} else {
			w.Header().Set("Alt-Svc", "clear")
		}
		next.ServeHTTP(w, req)
	})
}

func (r *rustHTTP3Runtime) shutdown(ctx context.Context) error {
	if r.stopping.Swap(true) {
		return nil
	}
	r.ready.Store(false)
	http3CompanionUp.Set(0)

	r.mu.Lock()
	control := r.control
	cmd := r.cmd
	r.mu.Unlock()
	if control != nil {
		_, _ = io.WriteString(control, "SHUTDOWN\n")
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	serverErr := r.internalServer.Shutdown(ctx)
	listenerErr := r.bridgeListener.Close()
	r.cancel()
	return errors.Join(serverErr, listenerErr)
}
