package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/integration/gen"
	"github.com/navidrome/navidrome/core/rustworker"
)

var errBodyTooLarge = errors.New("integration request body too large")

type grpcClient struct {
	proc     *rustworker.GRPCProcess
	client   gen.OutboundClient
	mu       sync.Mutex
	closed   bool
	inflight sync.WaitGroup
	onDead   func()
}

func startGRPCClient(ctx context.Context) (*grpcClient, error) {
	binary, err := Resolve()
	if err != nil {
		return nil, err
	}
	listen := grpcListenAddr()
	proc, err := rustworker.StartGRPC(ctx, binary, listen, nil)
	if err != nil {
		return nil, err
	}
	client := gen.NewOutboundClient(proc.Conn)
	healthCtx, healthCancel := context.WithTimeout(context.Background(), rustworker.DefaultGRPCDialTimeout)
	defer healthCancel()
	if _, err := client.Health(healthCtx, &gen.HealthRequest{}); err != nil {
		proc.Close()
		return nil, fmt.Errorf("integration worker health: %w", err)
	}
	c := &grpcClient{proc: proc, client: client}
	go c.watchProcess()
	return c, nil
}

func (c *grpcClient) watchProcess() {
	if c.proc == nil || c.proc.Cmd == nil {
		return
	}
	_ = c.proc.Wait()
	c.mu.Lock()
	closed := c.closed
	onDead := c.onDead
	c.mu.Unlock()
	if !closed && onDead != nil {
		onDead()
	}
}

func (c *grpcClient) roundTrip(ctx context.Context, dest Destination, req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("integration gRPC client closed")
	}
	client := c.client
	c.inflight.Add(1)
	c.mu.Unlock()
	defer c.inflight.Done()
	if client == nil {
		return nil, errors.New("integration gRPC client closed")
	}

	var body []byte
	if req.Body != nil {
		var err error
		body, err = readLimitedBody(req.Body, maxRequestBody(dest))
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
			headers[k] = strings.Join(v, ",")
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
		return nil, workerResponseError(resp.GetError())
	}

	respBody := resp.GetBody()
	if int64(len(respBody)) > maxResponseBody(dest) {
		return nil, fmt.Errorf("integration response exceeds %d bytes", maxResponseBody(dest))
	}

	out := &http.Response{
		StatusCode: int(resp.GetStatus()),
		Header:     make(http.Header, len(resp.GetHeaders())),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
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
	if ms := resp.GetRetryAfterMs(); ms > 0 {
		secs := (ms + 999) / 1000
		if secs < 1 {
			secs = 1
		}
		out.Header.Set("Retry-After", strconv.Itoa(int(secs)))
	}
	out.ContentLength = int64(len(respBody))
	return out, nil
}

func readLimitedBody(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w (%d bytes)", errBodyTooLarge, limit)
	}
	return data, nil
}

func (c *grpcClient) sign(ctx context.Context, params map[string]string, secret string) (string, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return "", errors.New("integration gRPC client closed")
	}
	client := c.client
	c.inflight.Add(1)
	c.mu.Unlock()
	defer c.inflight.Done()
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

func grpcListenAddr() string {
	if listen := strings.TrimSpace(os.Getenv("ND_INTEGRATIONGRPCLISTEN")); listen != "" {
		return listen
	}
	if listen := strings.TrimSpace(conf.Server.Integration.Listen); listen != "" {
		return listen
	}
	return rustworker.DefaultListenAddr("navidrome-integration")
}

func workerResponseError(msg string) error {
	if strings.Contains(msg, "circuit open") {
		return fmt.Errorf("%w: %s", errCircuitOpen, strings.TrimPrefix(msg, "circuit open for "))
	}
	return fmt.Errorf("integration worker: %s", msg)
}

func isWorkerCircuitOpen(err error) bool {
	return errors.Is(err, errCircuitOpen)
}

func (c *grpcClient) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.inflight.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.proc != nil {
		c.proc.Close()
		c.proc = nil
	}
	c.client = nil
}
