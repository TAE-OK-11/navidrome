# Rust worker benchmarks

Criterion benchmarks live in standalone crates so Docker layer caching for worker
binaries is unaffected. The workspace pins Rust **1.98** via `rust/rust-toolchain.toml`.

```bash
cd rust/benchmarks/search && cargo bench
cd rust/benchmarks/scanner && cargo bench
cd rust/benchmarks/metadata && cargo bench
```

## HTTP/3 inherited bridge (Go)

The tokio-quiche companion forwards requests to Go over an inherited HTTP/2 unix
socket. These benchmarks exercise that bridge on Linux:

```bash
go test -tags netgo,sqlite_fts5 -bench='BenchmarkHTTP2InheritedBridge' -run='^$' ./server/...
```

`BenchmarkHTTP2InheritedBridgeMediaStream` mirrors the 64 KiB chunked transcode
streaming path and validates full payload delivery through `ioutils.Copy`.

For QUIC rollout comparisons against H1/H2, use `cmd/http3bench` with baseline and
candidate HTTPS endpoints.
