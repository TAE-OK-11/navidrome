# Profile-guided optimization

Navidrome builds accept Go's `-pgo` build option through the `PGO` Make variable and Docker build argument.

The default is `PGO=auto`. Go automatically uses `default.pgo` when that file exists beside Navidrome's main package in the repository root. Without that file, the build remains a normal optimized Go build.

Use a representative production CPU pprof profile rather than a microbenchmark or a mostly idle profile:

```sh
cp /path/to/representative-cpu.pprof default.pgo
make pgo-build
make docker-image PGO=default.pgo
```

Disable PGO explicitly when diagnosing build or performance differences:

```sh
make build PGO=off
make docker-image PGO=off
```

PGO optimizes the complete Go program, including standard-library and dependency packages. Keep the profile under review like any other build input and refresh it from representative workloads as the application changes.

Go does not expose a traditional whole-program LTO switch for Go code. Passing C/C++ `-flto` flags to the external linker would only affect compatible C objects, would not optimize Go packages together, and would make Navidrome's multi-platform CGO cross-builds less reliable, so LTO is intentionally not enabled.
