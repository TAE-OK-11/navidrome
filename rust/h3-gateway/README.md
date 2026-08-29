# Navidrome tokio-quiche HTTP/3 companion

`navidrome-h3` owns the public UDP socket and the complete QUIC lifecycle. It
converts H3 request/response streams to a private authenticated HTTP/2 cleartext
connection that is created and supervised by the Go process. No QUIC packets or
packet buffers cross an FFI boundary.

The private bridge authenticates every request with a per-process random token
and accepts only loopback peers. It strips spoofable forwarding/internal
headers, then restores the original QUIC peer address, authority, HTTPS state
and HTTP/3 protocol metadata before invoking the existing Go middleware chain.

The production provider is `tokio-quiche`. The JBS image
installs the companion at `/app/navidrome-h3`; a non-image deployment can set
`HTTP3GatewayPath`.

Important settings:

- `EnableHTTP3`: starts the UDP listener; TLS certificate and key are required.
- `HTTP3AltSvcMaxAge`: defaults to five minutes during migration.
- `HTTP3QlogDir`: opt-in per-connection qlog output. Apply external retention
  because qlogs are intentionally not enabled by default.
- `HTTP3MaxConnections`: global connection admission limit (default 4096).
- `HTTP3MaxConnectionsPerIP`: per-client active limit (default 128).
- `HTTP3MaxInFlightRequests`: global request limit across all H3 connections
  (default 1024). Excess streams receive 503 immediately instead of consuming
  unbounded proxy tasks and memory.
- `HTTP3ConnectionRatePerSecond` and `HTTP3ConnectionBurst`: per-IP token bucket
  defaults of 50/s and 100 additional burst connections.
- `HTTP3CongestionControl`: defaults to `bbr2`, using quiche's gcongestion
  BBRv2 implementation. `cubic` and `reno` remain explicit rollback choices.
- `NAVIDROME_H3_THREADS`: optional Tokio worker override. By default the
  companion uses the available CPU parallelism capped at four workers.

0-RTT early data is enabled for resumed connections. Only GET and HEAD that
are not auth-session paths are accepted before the handshake completes; other
methods receive HTTP 425 Too Early and should retry. Session resumption itself
remains available either way. H3 extended CONNECT is not advertised and CONNECT
is answered with 501; WebSocket clients continue using the existing HTTP/1.1
upgrade path. Range, streaming request and response bodies, and SSE use bounded
async streams with transport backpressure.

On Linux the listener probes and enables the available UDP GSO/GRO,
`SO_TXTIME`, overflow accounting and PMTU capabilities. It also uses 7 MiB
socket buffer requests, BBRv2 by default, pacing, HyStart, PMTU discovery,
stateless Retry, a three-times amplification limit, and conservative
connection-ID/path state. The internal Hyper client is H2-only with adaptive
flow-control windows.

Prometheus exports `navidrome_http3_companion_up`,
`navidrome_http3_companion_restarts_total`, and
`navidrome_http3_bridge_rejected_total`. Existing Navidrome HTTP metrics also
cover requests arriving through H3. When the companion is unavailable,
H1/H2 stay up and emit `Alt-Svc: clear`; unexpected companion exits are
restarted with bounded exponential backoff.

## Docker and reverse proxies

`Dockerfile.jbs` exposes both `4533/tcp` and `4533/udp`. HTTP/3 clients connect
to the **UDP** port on the same number as HTTPS; the Rust companion terminates
QUIC there. The internal HTTP/2 bridge to Go is not visible on the public
network.

When using Caddy, Traefik, or nginx in front of Navidrome:

- Map **TCP** for HTTPS (H1/H2) through the proxy as usual.
- Map **UDP 4533** directly to Navidrome (or disable `EnableHTTP3` and let the
  proxy own HTTP/3). A TCP-only reverse proxy cannot carry QUIC.

The `contrib/docker-compose/` examples publish both `4533/tcp` and `4533/udp`
on the navidrome service for direct H3 access alongside proxied H1/H2.

## Validation and rollout

The Rust unit tests cover header sanitization, CONNECT detection, congestion
control validation, and admission. `TestRustHTTP3EndToEnd` uses a real H3 client
and validates repeated requests without 425, Range/206 seek, a large streamed
request body, SSE, and graceful shutdown across the Rust-to-Go boundary.
`Dockerfile.jbs` runs both unit and end-to-end tests.

For deployment comparison, run the benchmark client against isolated but
otherwise identical tokio-quiche instances:

```sh
NAVIDROME_BENCH_AUTHORIZATION='Bearer ...' go run ./cmd/http3bench \
  -baseline https://baseline.example:4533 \
  -candidate https://candidate.example:4533 \
  -api-path '/rest/ping.view?f=json' \
  -range-path '/rest/stream.view?id=TRACK_ID' \
  -artwork-path '/rest/getCoverArt.view?id=ALBUM_ID' \
  -duration 10m -concurrency 64
```

The command exits non-zero on any candidate 425 or Range failure, an error-rate
increase above 0.1 percentage point, more than 10% p95 regression, more than
15% p99 regression, or more than 10% request-throughput regression. Repeat
under representative RTT/loss using the deployment's traffic impairment
tooling, and separately record server CPU, RSS, UDP drops and qlogs.
