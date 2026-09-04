package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/integration/gen"
	"github.com/navidrome/navidrome/core/rustworker"
)

type grpcClient struct {
	proc   *rustworker.GRPCProcess
	client gen.OutboundClient
	mu     sync.Mutex
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
	return &grpcClient{proc: proc, client: client}, nil
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
		return nil, workerResponseError(resp.GetError())
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
	defer c.mu.Unlock()
	if c.proc != nil {
		c.proc.Close()
		c.proc = nil
	}
	c.client = nil
}
