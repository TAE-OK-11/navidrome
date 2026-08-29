# Rust worker benchmarks

Criterion benchmarks live in standalone crates so Docker layer caching for worker
binaries is unaffected.

```bash
cd rust/benchmarks/search && cargo +1.98.0 bench
cd rust/benchmarks/scanner && cargo +1.98.0 bench
cd rust/benchmarks/metadata && cargo +1.98.0 bench
```
