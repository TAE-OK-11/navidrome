package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/core/lifecycle"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/utils/httpclient"
	"github.com/navidrome/navidrome/utils/singleton"
)

var errCircuitOpen = errors.New("integration circuit open")

// Gateway is the hub for all outbound HTTP. Adapters no longer own
// point-to-point http.Client instances: they share this RoundTripper, which
// prefers the Rust gRPC worker (async I/O, pooling, signing) and falls back
// to Go net/http when the worker is unavailable. DestArtwork uses a separate
// SSRF-safe fallback so image CDNs never skip the private-IP checks.
type Gateway struct {
	fallback        http.RoundTripper
	artworkFallback http.RoundTripper
	breakers        sync.Map // Destination -> *circuitBreaker
	grpc            *grpcClient
}

func Get() *Gateway {
	return singleton.GetInstance(func() *Gateway {
		return newGateway()
	})
}

func newGateway() *Gateway {
	g := &Gateway{
		fallback:        httpclient.NewTransport(nil),
		artworkFallback: httpclient.NewTransport(newSSRFTransport()),
	}
	if conf.Server.Integration.Enabled {
		if client, err := startGRPCClient(context.Background()); err != nil {
			if !errors.Is(err, rustworker.ErrSkippedInTests) {
				log.Warn("Rust integration gRPC worker unavailable; using Go HTTP fallback", err)
			}
		} else {
			g.grpc = client
			log.Info("Outbound HTTP routed through Rust gRPC integration worker")
		}
	}
	lifecycle.Register(g)
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

// ArtworkHTTPClient fetches attacker-influenced image URLs through DestArtwork.
// Production prefers the Rust worker (SSRF in the worker); tests and worker
// outages use the Go SSRF transport.
func (g *Gateway) ArtworkHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     destTransport{g: g, dest: DestArtwork},
		CheckRedirect: artworkRedirectCheck,
	}
}

func ArtworkHTTPClient(timeout time.Duration) *http.Client {
	return Get().ArtworkHTTPClient(timeout)
}

type destTransport struct {
	g    *Gateway
	dest Destination
}

func (t destTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.g.roundTripDest(req, t.dest)
}

func (g *Gateway) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("nil request")
	}
	return g.roundTripDest(req, DestinationFromHost(req.URL.Host))
}

func (g *Gateway) roundTripDest(req *http.Request, dest Destination) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if dest == "" {
		dest = DestUnknown
	}
	breaker := g.breaker(dest)
	if !breaker.allow() {
		return nil, fmt.Errorf("%w: %s", errCircuitOpen, dest)
	}

	fallback := g.fallback
	if dest == DestArtwork && g.artworkFallback != nil {
		fallback = g.artworkFallback
	}

	var (
		resp *http.Response
		err  error
	)
	if g.grpc != nil && dest != DestUnknown {
		resp, err = g.grpc.roundTrip(req.Context(), dest, req)
	} else {
		resp, err = fallback.RoundTrip(req)
	}
	if err != nil {
		if isWorkerCircuitOpen(err) {
			return nil, err
		}
		breaker.failure()
		if g.grpc != nil && dest != DestUnknown {
			log.Trace(req.Context(), "gRPC outbound failed, falling back to Go HTTP", "dest", dest, err)
			resp, err = fallback.RoundTrip(req)
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
