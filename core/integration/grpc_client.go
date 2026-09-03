package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/core/integration/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const grpcDialTimeout = 5 * time.Second

type grpcClient struct {
	cmd    *exec.Cmd
	conn   *grpc.ClientConn
	client gen.OutboundClient
	mu     sync.Mutex
}

func startGRPCClient(ctx context.Context) (*grpcClient, error) {
	binary, err := Resolve()
	if err != nil {
		return nil, err
	}
	listen := strings.TrimSpace(os.Getenv("ND_INTEGRATIONGRPCLISTEN"))
	if listen == "" {
		listen = "unix:" + filepath.Join(os.TempDir(), fmt.Sprintf("navidrome-integration-%d.sock", os.Getpid()))
	}
	if path, ok := strings.CutPrefix(listen, "unix:"); ok {
		_ = os.Remove(path)
	}

	cmd := exec.CommandContext(ctx, binary, "--grpc-worker", "--listen", listen) //nolint:gosec // administrator-controlled worker
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("integration worker stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting integration worker: %w", err)
	}

	readyCtx, cancel := context.WithTimeout(ctx, grpcDialTimeout)
	defer cancel()
	addr, err := waitReady(readyCtx, stdout)
	if err != nil {
		rustworker.Kill(cmd)
		_ = cmd.Wait()
		return nil, err
	}

	dialAddr, dialOpts, err := grpcDialTarget(addr)
	if err != nil {
		rustworker.Kill(cmd)
		_ = cmd.Wait()
		return nil, err
	}
	conn, err := grpc.NewClient(dialAddr, dialOpts...)
	if err != nil {
		rustworker.Kill(cmd)
		_ = cmd.Wait()
		return nil, fmt.Errorf("dialing integration worker: %w", err)
	}

	client := gen.NewOutboundClient(conn)
	healthCtx, healthCancel := context.WithTimeout(context.Background(), grpcDialTimeout)
	defer healthCancel()
	if _, err := client.Health(healthCtx, &gen.HealthRequest{}); err != nil {
		_ = conn.Close()
		rustworker.Kill(cmd)
		_ = cmd.Wait()
		return nil, fmt.Errorf("integration worker health: %w", err)
	}

	go func() {
		_ = cmd.Wait()
	}()

	return &grpcClient{cmd: cmd, conn: conn, client: client}, nil
}

func waitReady(ctx context.Context, stdout io.Reader) (string, error) {
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
		return "", fmt.Errorf("waiting for integration worker: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return "", fmt.Errorf("reading integration worker ready line: %w", r.err)
		}
		line := strings.TrimSpace(r.line)
		addr, ok := strings.CutPrefix(line, "READY ")
		if !ok || addr == "" {
			return "", fmt.Errorf("unexpected integration worker banner %q", line)
		}
		return addr, nil
	}
}

func grpcDialTarget(addr string) (string, []grpc.DialOption, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}
	if path, ok := strings.CutPrefix(addr, "unix:"); ok {
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}))
		return "passthrough:///unix", opts, nil
	}
	return addr, opts, nil
}

func (c *grpcClient) roundTrip(ctx context.Context, dest Destination, req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return nil, errors.New("integration gRPC client closed")
	}

	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	timeoutMs := int32(0)
	if deadline, ok := ctx.Deadline(); ok {
		timeoutMs = int32(time.Until(deadline).Milliseconds())
		if timeoutMs < 1 {
			timeoutMs = 1
		}
	}

	headers := make(map[string]string, len(req.Header))
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	resp, err := client.Call(ctx, &gen.HttpRequest{
		Destination: string(dest),
		Method:      req.Method,
		Url:         req.URL.String(),
		Headers:     headers,
		Body:        body,
		TimeoutMs:   timeoutMs,
	})
	if err != nil {
		return nil, err
	}
	if resp.GetError() != "" && resp.GetStatus() == 0 {
		return nil, fmt.Errorf("integration worker: %s", resp.GetError())
	}

	out := &http.Response{
		StatusCode: int(resp.GetStatus()),
		Header:     make(http.Header, len(resp.GetHeaders())),
		Body:       io.NopCloser(bytes.NewReader(resp.GetBody())),
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		Request:    req,
	}
	if out.StatusCode == 0 {
		out.StatusCode = http.StatusOK
	}
	for k, v := range resp.GetHeaders() {
		out.Header.Set(k, v)
	}
	out.ContentLength = int64(len(resp.GetBody()))
	return out, nil
}

func (c *grpcClient) sign(ctx context.Context, params map[string]string, secret string) (string, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return "", errors.New("integration gRPC client closed")
	}
	resp, err := client.Sign(ctx, &gen.SignRequest{Params: params, Secret: secret})
	if err != nil {
		return "", err
	}
	if resp.GetError() != "" {
		return "", fmt.Errorf("integration sign: %s", resp.GetError())
	}
	return resp.GetApiSig(), nil
}

func (c *grpcClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	rustworker.Kill(c.cmd)
	c.client = nil
}
