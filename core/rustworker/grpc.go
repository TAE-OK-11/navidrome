package rustworker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/lifecycle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const DefaultGRPCDialTimeout = 5 * time.Second

// DefaultGRPCHealthTimeout bounds the post-dial Health RPC. READY already
// proves the listener is up; health only confirms the service is registered.
const DefaultGRPCHealthTimeout = 2 * time.Second

const maxGRPCMsgSize = 64 << 20

// Local IPC window sizes: default HTTP/2 windows (64KiB) add RTT stalls on
// bulk metadata/search payloads over unix sockets. 1–2 MiB keeps large unary
// RPCs from waiting on window updates without ballooning RAM.
const (
	grpcStreamWindowSize = 1 << 20 // 1 MiB
	grpcConnWindowSize   = 2 << 20 // 2 MiB
)

// ErrSkippedInTests is returned when a gRPC worker would be spawned from a
// `go test` process. Child workers hold stdout pipes open and make the test
// binary hang on "WaitDelay expired before I/O complete".
var ErrSkippedInTests = errors.New("gRPC worker not started in Go tests")

// Shared across dials: insecure.NewCredentials() allocates; local workers all
// use the same plaintext transport.
var insecureLocal = insecure.NewCredentials()

// DefaultListenAddr returns a process-local unix socket (TCP on Windows).
func DefaultListenAddr(prefix string) string {
	return "unix:" + filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d.sock", prefix, os.Getpid()))
}

// GRPCProcess is a Rust worker serving tonic on a unix/TCP socket.
type GRPCProcess struct {
	Cmd  *exec.Cmd
	Conn *grpc.ClientConn
	Addr string

	waitOnce sync.Once
	waitErr  error
}

// StartGRPC launches binary with --grpc-worker --listen, waits for READY, and dials.
// The worker process outlives ctx: ctx only bounds the READY wait. Callers that
// need a temporary probe must Close the returned process themselves. This lets
// startup preflight Adopt the same process into ManagedGRPC without the
// preflight timeout cancelling CommandContext and killing the companion.
func StartGRPC(ctx context.Context, binary string, listen string, extraEnv []string) (*GRPCProcess, error) {
	if skipGRPCWorkerInTests() {
		return nil, ErrSkippedInTests
	}
	if listen == "" {
		listen = DefaultListenAddr("navidrome-worker")
	}
	UnlinkUnixListen(listen)

	cmd := exec.Command(binary, "--grpc-worker", "--listen", listen) //nolint:gosec // administrator-controlled worker
	prepareCmd(cmd)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening worker stdout: %w", err)
	}
	if testing.Testing() {
		cmd.Stderr = io.Discard
	} else {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting gRPC worker %q: %w", binary, err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, DefaultGRPCDialTimeout)
	defer cancel()
	addr, err := WaitReady(readyCtx, stdout)
	if err != nil {
		Kill(cmd)
		_ = cmd.Wait()
		return nil, err
	}
	go func() { _, _ = io.Copy(io.Discard, stdout) }()

	conn, err := DialGRPC(addr)
	if err != nil {
		Kill(cmd)
		_ = cmd.Wait()
		return nil, err
	}

	proc := &GRPCProcess{Cmd: cmd, Conn: conn, Addr: addr}
	lifecycle.Register(proc)
	return proc, nil
}

func skipGRPCWorkerInTests() bool {
	if !testing.Testing() {
		return false
	}
	return strings.TrimSpace(os.Getenv("ND_GRPCWORKERINTESTS")) == ""
}

// UnlinkUnixListen removes a stale unix socket we previously created under TempDir.
func UnlinkUnixListen(listen string) {
	path, ok := strings.CutPrefix(listen, "unix:")
	if !ok || path == "" {
		return
	}
	cleaned := filepath.Clean(path)
	tmp := filepath.Clean(os.TempDir())
	if cleaned != tmp && !strings.HasPrefix(cleaned, tmp+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(cleaned) //nolint:gosec // G703: stale unix socket we created under TempDir
}

// Wait reaps the worker process exactly once. Safe for concurrent callers
// (ManagedGRPC watch + Close, integration watch + close).
func (p *GRPCProcess) Wait() error {
	if p == nil {
		return nil
	}
	p.waitOnce.Do(func() {
		if p.Cmd != nil {
			p.waitErr = p.Cmd.Wait()
		}
	})
	return p.waitErr
}

func (p *GRPCProcess) Close() {
	if p == nil {
		return
	}
	if p.Conn != nil {
		_ = p.Conn.Close()
		p.Conn = nil
	}
	Kill(p.Cmd)
	_ = p.Wait()
}

// WaitReady reads the worker's "READY <addr>" banner from stdout.
func WaitReady(ctx context.Context, stdout io.Reader) (string, error) {
	reader := bufio.NewReaderSize(stdout, 256)
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- result{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for gRPC worker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return "", fmt.Errorf("reading gRPC worker ready line: %w", r.err)
		}
		line := strings.TrimSpace(r.line)
		addr, ok := strings.CutPrefix(line, "READY ")
		if !ok || addr == "" {
			return "", fmt.Errorf("unexpected gRPC worker banner %q", line)
		}
		return addr, nil
	}
}

func DialGRPC(addr string) (*grpc.ClientConn, error) {
	// Local companion IPC: no TLS, no compression, soft keepalive (dead unix
	// peers are noticed via socket close; idle pings only add chatter and can
	// trip server min-time enforcement on long-idle workers).
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecureLocal),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                2 * time.Minute,
			Timeout:             5 * time.Second,
			PermitWithoutStream: false,
		}),
		grpc.WithInitialWindowSize(grpcStreamWindowSize),
		grpc.WithInitialConnWindowSize(grpcConnWindowSize),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGRPCMsgSize),
			grpc.MaxCallSendMsgSize(maxGRPCMsgSize),
		),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  100 * time.Millisecond,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   2 * time.Second,
			},
			MinConnectTimeout: DefaultGRPCDialTimeout,
		}),
	}
	dialAddr := addr
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: DefaultGRPCDialTimeout}
			return d.DialContext(ctx, "unix", path)
		}))
		dialAddr = "passthrough:///unix"
	}
	conn, err := grpc.NewClient(dialAddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing gRPC worker: %w", err)
	}
	return conn, nil
}
