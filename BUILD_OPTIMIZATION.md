# Build optimization review

Production packaging for this fork is `Dockerfile.jbs`, published by the JBS GHCR
workflow. This review covers the build pipeline and dependency selection; it does
not claim measured application speedups from enabling compiler flags.

## LTO and PGO

| Component | Current final build | Training |
| --- | --- | --- |
| Go application | Go PGO plus external CGO link | Twenty application benchmarks, with shorter runs for light operations |
| CGO SQLite and native codecs/allocator | Clang full LTO and LLD | Go CPU profiles exercise their callers; they are **not** native LLVM PGO profiles |
| Rust gRPC workers | `release-fat`, full LTO, one codegen unit | Instrumented Criterion workloads; optional Go integration exercises |
| Rust HTTP/3 gateway | Full LTO | No dedicated PGO workload yet |

These settings already existed. Full LTO is retained; its advantage over ThinLTO
must be measured on actual workloads, not assumed from the flag name. PGO is only
as useful as its workload coverage. Production Go profiles remain preferable to
synthetic benchmarks when representative captures become available, as explained
in the [Go PGO documentation](https://go.dev/doc/pgo).

The Go trainer now explicitly disables automatic PGO during collection, writes
its test executables to the writable profile directory, and removes them after
each workload. This supports read-only Docker source mounts and avoids accumulating
large `.test` artifacts. A nonempty profile alone no longer counts as successful
training: a completed benchmark result is required, preventing skipped or renamed
benchmarks from silently supplying startup-only profiles.

Rust PGO resolves `llvm-profdata` from the active compiler's `llvm-tools-preview`
component. It no longer assumes the distribution LLVM matches rustc. All Cargo
training/final commands explicitly select the host target, isolating build
scripts and proc macros from instrumentation. Profile and target directories are
absolute across standalone benchmark crates. This follows the
[Rust PGO workflow](https://doc.rust-lang.org/rustc/profile-guided-optimization.html).
The final script output is now under `pgo-final/<host-triple>/release-fat`.

Training still uses portable CPU instructions and a cheaper LTO-off generation
profile; final artifacts retain Zen 3 tuning and full LTO. Differences between
training and final code generation and sparse coverage of cold worker paths can
reduce profile usefulness. This review does not claim all Rust worker methods or
native C functions have trained profiles.

## Docker and CI

The build context excludes local Rust targets, Node modules, Git objects, caches,
test executables and raw LLVM profiles. JBS image triggers now include all release
scripts, resources, the workspace Rust toolchain and `.dockerignore`; duplicate
benchmark path entries were removed. Fast build-script regressions run in CI.

JBS source acquisition now fetches the requested branch/tag/commit directly,
removing the preceding remote branch/tag probes. The standard Dockerfile fetches
only the pinned xx tool commit rather than its entire history, mounts the actual
UI bundle stage, avoids recommended APT packages and drops the unused SQLite CLI
(the application uses its compiled SQLite driver). Jukebox mpv remains installed.

Optional cache-statistics and Python-cache cleanup failures can still be ignored,
but they no longer hide failures from native configure/make/install/strip or
runtime dependency installation. A regression test executes the actual Docker RUN
chains with failing tool stubs. Rust installs a minimal toolchain plus matching
LLVM profiling tools.

The upstream-style standard Dockerfile remains a legacy distribution path: unlike
JBS it does not package this fork's mandatory Rust companions, and its cross-target
PGO training does not provision them. Fixing the UI mount is not a claim that this
path now produces a complete runnable fork image. Production uses JBS; full
portable multi-architecture companion packaging is separate remaining work.

## Dependency selection

The runtime integration worker and its PGO benchmark resolve matching versions:

- [reqwest 0.13.5](https://github.com/seanmonstar/reqwest/releases/tag/v0.13.5):
  removes unnecessary clones, reuses read-timeout timers, and corrects timeout
  classification and proxy credential selection.
- [rustls 0.23.44](https://github.com/rustls/rustls/releases/tag/v/0.23.44):
  maintenance fixes for ECH rejection certificate verification and keylog file
  permissions. This is not a claim that this fork enables ECH or key logging.
- base64 0.23.1 is reqwest's required transitive update; older base64 remains where
  other dependencies require it. Unrelated lockfile packages were retained.

Selected Go networking, protobuf, router, compression and SQLite dependencies
already had no newer patch within their current minor lines during this review.
No blanket major-version migration or unverified base-image bump was performed.

## Validation scope

`release/test_pgo_scripts.py` tests orchestration using fake compiler commands:
completed workloads, skipped workloads, failures, scratch cleanup, LLVM selection
and explicit target propagation. These tests verify command contracts, not real
profile quality or final machine code. Shell syntax and locked Cargo metadata are
also checked locally. Full instrumented builds and runtime regressions remain in
GitHub Actions; do not infer their success from these lightweight local checks.
