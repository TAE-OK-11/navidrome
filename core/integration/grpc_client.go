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
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/integration/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"google.golang.org/grpc"
)

var errBodyTooLarge = errors.New("integration request body too large")

// grpcClient delegates process ownership to the shared worker supervisor.
// Outbound HTTP is never automatically replayed: a lost reply may already
// have produced a remote side effect (scrobble, webhook, etc.).
type grpcClient struct {
	manager *rustworker.ManagedGRPC
}

func startGRPCClient(ctx context.Context) (*grpcClient, error) {
	manager := rustworker.NewManagedGRPC(rustworker.ManagedGRPCConfig{
		Name:    "integration",
		Listen:  grpcListenAddr(),
		Resolve: Resolve,
		Health: func(ctx context.Context, conn *grpc.ClientConn) error {
			_, err := gen.NewOutboundClient(conn).Health(ctx, &gen.HealthRequest{})
			return err
		},
	})
	if _, err := manager.ConnContext(ctx); err != nil {
		manager.Close()
		return nil, err
	}
	return &grpcClient{manager: manager}, nil
}

func (c *grpcClient) roundTrip(ctx context.Context, dest Destination, req *http.Request) (*http.Response, error) {
	conn, err := c.manager.ConnContext(ctx)
	if err != nil {
		return nil, err
	}
	client := gen.NewOutboundClient(conn)

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
	c.manager.Close()
}
