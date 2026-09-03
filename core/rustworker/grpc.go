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
	"testing"
	"time"

	"github.com/navidrome/navidrome/core/lifecycle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const DefaultGRPCDialTimeout = 5 * time.Second

const maxGRPCMsgSize = 64 << 20

// ErrSkippedInTests is returned when a gRPC worker would be spawned from a
// `go test` process. Child workers hold stdout pipes open and make the test
// binary hang on "WaitDelay expired before I/O complete".
var ErrSkippedInTests = errors.New("gRPC worker not started in Go tests")

// DefaultListenAddr returns a process-local unix socket (TCP on Windows).
func DefaultListenAddr(prefix string) string {
	return "unix:" + filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d.sock", prefix, os.Getpid()))
}

// GRPCProcess is a Rust worker serving tonic on a unix/TCP socket.
type GRPCProcess struct {
	Cmd  *exec.Cmd
	Conn *grpc.ClientConn
	Addr string
}

// StartGRPC launches binary with --grpc-worker --listen, waits for READY, and dials.
func StartGRPC(ctx context.Context, binary string, listen string, extraEnv []string) (*GRPCProcess, error) {
	if skipGRPCWorkerInTests() {
		return nil, ErrSkippedInTests
	}
	if listen == "" {
		listen = DefaultListenAddr("navidrome-worker")
	}
	UnlinkUnixListen(listen)

	cmd := exec.CommandContext(ctx, binary, "--grpc-worker", "--listen", listen) //nolint:gosec // administrator-controlled worker
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

	go func() { _ = cmd.Wait() }()
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

func (p *GRPCProcess) Close() {
	if p == nil {
		return
	}
	if p.Conn != nil {
		_ = p.Conn.Close()
		p.Conn = nil
	}
	Kill(p.Cmd)
}

// WaitReady reads the worker's "READY <addr>" banner from stdout.
func WaitReady(ctx context.Context, stdout io.Reader) (string, error) {
	reader := bufio.NewReader(stdout)
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
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxGRPCMsgSize),
			grpc.MaxCallSendMsgSize(maxGRPCMsgSize),
		),
	}
	dialAddr := addr
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
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
