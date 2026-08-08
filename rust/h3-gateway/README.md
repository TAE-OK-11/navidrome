# Navidrome tokio-quiche HTTP/3 companion

`navidrome-h3` owns the public UDP socket and the complete QUIC lifecycle. It
converts H3 request/response streams to a private authenticated HTTP/2 cleartext
connection that is created and supervised by the Go process. No QUIC packets or
packet buffers cross an FFI boundary.

The private bridge authenticates every request with a per-process random token
and accepts only loopback peers. It strips spoofable forwarding/internal
headers, then restores the original QUIC peer address, authority, HTTPS state
and HTTP/3 protocol metadata before invoking the existing Go middleware chain.

The provider is selected with `HTTP3Provider = "tokio-quiche"` (the default
when HTTP/3 is enabled). The JBS image installs the companion at
`/app/navidrome-h3`; a non-image deployment can set `HTTP3GatewayPath`.

Important settings:

- `EnableHTTP3`: starts the UDP listener; TLS certificate and key are required.
- `HTTP3AltSvcMaxAge`: defaults to five minutes during migration.
- `HTTP3QlogDir`: opt-in per-connection qlog output. Apply external retention
  because qlogs are intentionally not enabled by default.
- `HTTP3MaxConnections`: global connection admission limit (default 4096).
- `HTTP3MaxConnectionsPerIP`: per-client active limit (default 128).
- `HTTP3ConnectionRatePerSecond` and `HTTP3ConnectionBurst`: per-IP token bucket
  defaults of 50/s and 100 additional burst connections.

0-RTT request data is always disabled. Session resumption remains available,
but Navidrome never receives an application request before handshake
completion, so the old route-level HTTP 425 behavior is gone. H3 extended
CONNECT is not advertised and CONNECT is answered with 501; WebSocket clients
continue using the existing HTTP/1.1 upgrade path. Range, streaming request and
response bodies, and SSE use bounded async streams with transport backpressure.

On Linux the listener probes and enables the available UDP GSO/GRO,
`SO_TXTIME`, overflow accounting and PMTU capabilities. It also uses 7 MiB
socket buffer requests, paced Cubic with HyStart, PMTU discovery, stateless
Retry, a three-times amplification limit, and conservative connection-ID/path
state. The internal Hyper client is H2-only with adaptive flow-control windows.

Prometheus exports `navidrome_http3_companion_up`,
`navidrome_http3_companion_restarts_total`, and
`navidrome_http3_bridge_rejected_total`. Existing Navidrome HTTP metrics also
cover requests arriving through H3. When the companion is unavailable,
H1/H2 stay up and emit `Alt-Svc: clear`; unexpected companion exits are
restarted with bounded exponential backoff.

## Validation and rollout

The Rust unit tests cover header sanitization, CONNECT detection and admission.
`TestRustHTTP3EndToEnd` uses a real quic-go H3 client and validates repeated
requests without 425, Range/206 seek, a large streamed request body, SSE, and
graceful shutdown across the Rust-to-Go boundary. `Dockerfile.jbs` runs both
unit and end-to-end tests.

For deployment comparison, run the benchmark client against isolated but
otherwise identical quic-go and tokio-quiche instances:

```sh
NAVIDROME_BENCH_AUTHORIZATION='Bearer ...' go run ./cmd/http3bench \
  -baseline https://quic-go.example:4533 \
  -candidate https://tokio-quiche.example:4533 \
  -api-path '/rest/ping.view?f=json' \
  -range-path '/rest/stream.view?id=TRACK_ID' \
  -artwork-path '/rest/getCoverArt.view?id=ALBUM_ID' \
  -duration 10m -concurrency 64
```

The command exits non-zero on any candidate 425 or Range failure, an error-rate
increase above 0.1 percentage point, more than 10% p95 regression, more than
15% p99 regression, or more than 10% request-throughput regression. Repeat
under representative RTT/loss using
the deployment's traffic impairment tooling, and separately record server CPU,
RSS, UDP drops and qlogs. Keep `HTTP3Provider = "quic-go"` as the rollback until
the candidate passes sustained canaries and long-connection soak tests; only
then remove the legacy provider and Go dependency.
