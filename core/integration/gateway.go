package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/httpclient"
	"github.com/navidrome/navidrome/utils/singleton"
)

var errCircuitOpen = errors.New("integration circuit open")

// Gateway is the hub for all outbound HTTP. Adapters no longer own
// point-to-point http.Client instances: they share this RoundTripper, which
// prefers the Rust gRPC worker (async I/O, pooling, signing) and falls back
// to Go net/http when the worker is unavailable.
type Gateway struct {
	fallback http.RoundTripper
	breakers sync.Map // Destination -> *circuitBreaker
	grpc     *grpcClient
}

func Get() *Gateway {
	return singleton.GetInstance(func() *Gateway {
		return newGateway()
	})
}

func newGateway() *Gateway {
	g := &Gateway{
		fallback: httpclient.NewTransport(nil),
	}
	if conf.Server.Integration.Enabled {
		if client, err := startGRPCClient(context.Background()); err != nil {
			log.Warn("Rust integration gRPC worker unavailable; using Go HTTP fallback", err)
		} else {
			g.grpc = client
			log.Info("Outbound HTTP routed through Rust gRPC integration worker")
		}
	}
	return g
}

func (g *Gateway) HTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = consts.DefaultHttpClientTimeOut
	}
	return &http.Client{Timeout: timeout, Transport: httpclient.NewTransport(g)}
}

func HTTPClient(timeout time.Duration) *http.Client {
	return Get().HTTPClient(timeout)
}

func (g *Gateway) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	dest := DestinationFromHost(req.URL.Host)
	breaker := g.breaker(dest)
	if !breaker.allow() {
		return nil, fmt.Errorf("%w: %s", errCircuitOpen, dest)
	}

	var (
		resp *http.Response
		err  error
	)
	if g.grpc != nil && dest != DestUnknown {
		resp, err = g.grpc.roundTrip(req.Context(), dest, req)
	} else {
		resp, err = g.fallback.RoundTrip(req)
	}
	if err != nil {
		breaker.failure()
		if g.grpc != nil && dest != DestUnknown {
			log.Trace(req.Context(), "gRPC outbound failed, falling back to Go HTTP", "dest", dest, err)
			resp, err = g.fallback.RoundTrip(req)
			if err != nil {
				return nil, err
			}
			if resp != nil && resp.StatusCode >= 500 {
				breaker.failure()
			} else {
				breaker.success()
			}
			return resp, nil
		}
		return nil, err
	}
	if resp != nil && resp.StatusCode >= 500 {
		breaker.failure()
	} else {
		breaker.success()
	}
	return resp, nil
}

func (g *Gateway) Sign(ctx context.Context, params map[string]string, secret string) string {
	if g.grpc != nil {
		sig, err := g.grpc.sign(ctx, params, secret)
		if err == nil && sig != "" {
			return sig
		}
	}
	return signAudioscrobbler(params, secret)
}

func Sign(ctx context.Context, params map[string]string, secret string) string {
	return Get().Sign(ctx, params, secret)
}

func (g *Gateway) breaker(dest Destination) *circuitBreaker {
	actual, _ := g.breakers.LoadOrStore(dest, &circuitBreaker{})
	return actual.(*circuitBreaker)
}

func (g *Gateway) Close() {
	if g.grpc != nil {
		g.grpc.close()
	}
}

type nopReadCloser struct {
	*bytes.Reader
}

func (nopReadCloser) Close() error { return nil }

func readCloser(body []byte) io.ReadCloser {
	return nopReadCloser{bytes.NewReader(body)}
}
