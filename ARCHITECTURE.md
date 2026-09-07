# Fork traffic and ownership

This describes the production fork, rather than upstream Navidrome. Entry points
and tests are the source of truth for configuration details.

## Request paths

| Path | Flow and ownership |
| --- | --- |
| HTTP/1.1 and HTTP/2 | `server/server.go` accepts TCP/TLS, dispatches public gRPC by content type or routes through Chi middleware, authentication, native/Subsonic/public handlers and Go services. |
| HTTP/3 | `rust/h3-gateway` owns UDP, QUIC, TLS and H3. A persistent inherited Unix socketpair carries multiplexed HTTP/2 to the same Go handler chain. Authentication and persistence stay shared with H1/H2. |
| Public gRPC | `server/publicgrpc` authenticates and directly invokes Go native/Subsonic handlers. `Open` streams bounded chunks with send backpressure; there is no local HTTP request round trip. HTTP abort signals become failed gRPC streams, never successful `Final` messages. |
| Playback | Go resolves authorization/media/transcode decisions, opens a file or transcoder cache/ffmpeg reader and serves it. Seekable files retain range/sendfile paths. Live output flushes headers and audio promptly. Empty/failed output after headers aborts the transfer rather than appending an API error or reporting clean completion. |
| Scanning | Rust walks the local filesystem and computes folder hashes, streaming changed folders or unchanged summaries to Go. Go processes folders, batches metadata extraction through Rust, and owns SQLite transactions, albums, playlists and scan progress. |
| Metadata/artwork | The Rust metadata worker owns Lofty extraction, tag cleaning, media mapping, lyrics, normalization and image work. Go owns library access policy, source selection and disk caches. External artwork uses the integration gateway and its restricted resolver. |
| Search | Go reads authorized SQLite data and batches document updates to Rust/Tantivy. Rust owns indexing/query execution and shared `fts-normalize` rules. Results return IDs for Go hydration and response generation. SQLite FTS normalization uses cached/batched metadata RPCs. |
| Outbound HTTP | Go adapters use `core/integration.Gateway`; enabled production networking runs through persistent Rust/reqwest clients with connection pools, destination breakers and artwork URL/DNS policy. Disabled-worker/test paths retain Go transports. Signing runs locally in Go before the HTTP call. |

## Changes and invariants

### Shared worker supervision

Metadata, scanner, search, API-key and outbound integration workers share
`core/rustworker.ManagedGRPC`. Connections are reused; local IPC remains
uncompressed protobuf over Unix sockets. READY and health checks bound startup.
Individual callers can cancel waiting for a shared startup without canceling
other callers' work. Published process/connection handles are immutable; exit is
published atomically and close/reap are idempotent. An old failed RPC cannot
invalidate a newer connection. Dead worker transports are closed instead of
reconnecting indefinitely.

The lifecycle registry now supports releasing registrations when resources are
closed. Both supervisors and processes participate in shutdown, so supervision
itself stops and historical worker generations are not retained indefinitely.
Indexing operations pin their complete batch sequence, commit and cleanup to one
worker connection. A worker crash fails the operation; per-batch retry cannot
silently commit only the tail of an incremental update on a replacement worker.
Read-only searches retain transport retry. Outbound integration no longer has a separate watcher/restart implementation or
an unbounded wait for in-flight HTTP calls on close. Remote HTTP operations are
not automatically replayed after an ambiguous transport failure.

### Backpressure and cancellation

The shared Rust `run_blocking` helper admits finite CPU/filesystem RPC work on
the async side before allocating blocking threads. Each process admits available
CPU parallelism, capped at 32 jobs. Canceled queued futures do not launch work;
already-started computations retain their permits until they actually finish.
Metadata, search and folder-hash RPCs use this policy. Long-lived traversal keeps
its separate streaming producer and bounded channel.

This follows Tokio's distinction between cancelable async admission and
[non-abortable started blocking tasks](https://docs.rs/tokio/1.53.1/tokio/task/fn.spawn_blocking.html).
It bounds active work, not total decoded request bytes or execution time of a
single decoder. Representative large-library measurements are still needed to
tune concurrency and batch sizes.

A scanner stream succeeds only after `DONE`. EOF without it is incomplete.
Partial walks are not automatically replayed. Every stream-adapter exit cancels
its child context. Traversal and pipeline errors stop finalization before any
unvisited folders/tracks are marked missing; later cleanup phases do not run.

### Less repeated work and buffering

Audioscrobbler signing uses the existing Go implementation and shared test
vector. This removes a signing RPC and its worker-health dependency from the
normal request path. The Rust signing protocol remains available for existing
clients and benchmarks.

Normalization caches successful empty results, coalesces duplicate groups in a
batch, frames cache keys unambiguously and evicts beyond 8,192 entries. Each
retained key/result pair is at most 4 KiB (32 MiB payload ceiling, plus cache
bookkeeping). Normalization rules remain solely in Rust.

Rust outbound responses are checked while receiving each chunk, including
responses without Content-Length, and accumulated directly into the protobuf
body Vec. This avoids the old complete-response buffer followed by another
full-body copy. The RPC remains unary, so bounded complete-body buffering is
still present at the Go/Rust boundary.

## Why retain these boundaries

Rust already owns the substantial parsing, image, indexing and QUIC workloads.
Go already owns authorization, SQLite transactions, application services and
stream/cache/ffmpeg lifetimes. Moving these again without workload evidence would
introduce substantial semantic risk. The H3-to-H2 bridge keeps all protocols on
one authorization and routing implementation. Replacing it requires measuring
its cost against duplicating that application layer or designing an FFI ABI.

The small signing operation justified moving to its caller. The other changes
address demonstrated races, unsafe partial completion, repeated RPCs and
unbounded resource retention rather than selecting a language by preference.

## Validation and follow-up

CI runs the full Go race suite with real Rust workers, lint/generated-file
checks, Rust crate tests, UI validation and the JBS image build. The shared
`grpc-listen` and `fts-normalize` crates are now explicit CI matrix entries.
Targeted tests cover concurrent worker close/recovery, caller cancellation,
registration release, scanner terminal-event/cleanup contracts, chunked body
limits, normalization caching and HTTP/gRPC abort translation.

Worthwhile next measurements/design work:

- Measure CPU/RSS and p50/p95/p99 latency on representative libraries and network
  conditions; no deployment throughput improvement is claimed from unit tests.
- Profile embedded media/lyrics JSON inside metadata protobuf messages before
  migrating their schema to typed protobuf fields.
- Measure unary artwork IPC memory and consider streaming or local-file handoff
  if large artwork dominates. Preserve decoder limits and URL/DNS restrictions.
- If workers become independently restarted remote services, extend connection
  pinning with explicit server generation/transaction IDs in the search protocol.
- Test injected worker crashes, cancellation and mixed scan/playback load with
  production-sized data; tune admission without starving interactive artwork.
