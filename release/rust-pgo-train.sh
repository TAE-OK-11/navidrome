#!/bin/sh
# Profile-guided optimization for Rust gRPC workers.
#
# 1. Build all workspace binaries with -Cprofile-generate
# 2. Run criterion benches (integration, metadata, scanner, search)
# 3. Optionally run Go integration gRPC tests against the instrumented worker
# 4. llvm-profdata merge -> merged.profdata
# 5. Rebuild all workspace gRPC workers with -Cprofile-use + release-fat LTO
#
# Environment:
#   RUST_PGO_DIR            profile output directory (default: /tmp/rust-pgo)
#   RUST_PGO_BENCH_TIME     criterion measurement time in seconds (default: 3)
#   RUST_PGO_GO_ROUNDS      Go integration test repetitions (default: 25, 0=skip)
#   RUST_PROFILE            final Cargo profile (default: release-fat)
#   CARGO_TARGET_DIR        cargo output directory
#   RUSTFLAGS               extra rustc flags (e.g. -C target-cpu=znver3)
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
PGO_DIR="${RUST_PGO_DIR:-/tmp/rust-pgo}"
BENCH_TIME="${RUST_PGO_BENCH_TIME:-3}"
GO_ROUNDS="${RUST_PGO_GO_ROUNDS:-25}"
PROFILE="${RUST_PROFILE:-release-fat}"
MERGED="${PGO_DIR}/merged.profdata"
TARGET_DIR="${CARGO_TARGET_DIR:-${ROOT}/rust/target}"
INSTR_TARGET="${TARGET_DIR}/pgo-instrument"
export CARGO_TARGET_DIR="${TARGET_DIR}"

case "${BENCH_TIME}" in
  *s) BENCH_TIME="${BENCH_TIME%s}" ;;
esac

cargo_cmd() {
  if command -v rustup >/dev/null 2>&1 && rustup run 1.98.0 cargo --version >/dev/null 2>&1; then
    rustup run 1.98.0 cargo "$@"
  else
    cargo "$@"
  fi
}

if ! command -v llvm-profdata >/dev/null 2>&1; then
  echo "[rust-pgo] llvm-profdata not found; install LLVM tools (clang/llvm)" >&2
  exit 1
fi

mkdir -p "${PGO_DIR}"
rm -f "${PGO_DIR}"/*.profraw "${MERGED}"

GEN_FLAGS="${RUSTFLAGS:-} -Cprofile-generate=${PGO_DIR} -Clto=off"

echo "[rust-pgo] phase 1: instrumented build (profile-generate, all workers)"
(
  cd "${ROOT}/rust"
  CARGO_TARGET_DIR="${INSTR_TARGET}" \
    RUSTFLAGS="${GEN_FLAGS}" \
    cargo_cmd build --locked --release --bins
)

echo "[rust-pgo] phase 2: collecting profiles (criterion ${BENCH_TIME}s)"
for bench in integration metadata scanner search; do
  bench_dir="${ROOT}/rust/benchmarks/${bench}"
  if [ ! -f "${bench_dir}/Cargo.toml" ]; then
    echo "[rust-pgo] skip missing bench crate ${bench}"
    continue
  fi
  bench_name="${bench}_hotpaths"
  echo "[rust-pgo]   bench ${bench}"
  (
    cd "${bench_dir}"
    CARGO_TARGET_DIR="${INSTR_TARGET}" \
      RUSTFLAGS="${GEN_FLAGS}" \
      cargo_cmd bench --bench "${bench_name}" -- --measurement-time "${BENCH_TIME}"
  )
done

INTEGRATION_BIN="${INSTR_TARGET}/release/navidrome-integration"
if [ "${GO_ROUNDS}" != "0" ] && [ -x "${INTEGRATION_BIN}" ] && command -v go >/dev/null 2>&1; then
  echo "[rust-pgo] phase 2b: Go gRPC integration tests (${GO_ROUNDS} rounds)"
  (
    cd "${ROOT}"
    i=1
    while [ "${i}" -le "${GO_ROUNDS}" ]; do
      ND_INTEGRATIONWORKERPATH="${INTEGRATION_BIN}" \
      ND_GRPCWORKERINTESTS=1 \
        go test -tags netgo,sqlite_fts5 -run '^TestIntegrationWorkerGRPC$' -count=1 ./core/integration/ >/dev/null
      i=$((i + 1))
    done
  )
fi

PROFRAW_COUNT="$(find "${PGO_DIR}" -name '*.profraw' 2>/dev/null | wc -l | tr -d ' ')"
if [ "${PROFRAW_COUNT}" = "0" ]; then
  echo "[rust-pgo] no .profraw files collected in ${PGO_DIR}" >&2
  exit 1
fi

echo "[rust-pgo] phase 3: merge ${PROFRAW_COUNT} profile(s)"
llvm-profdata merge -o "${MERGED}" "${PGO_DIR}"/*.profraw
test -s "${MERGED}"

USE_FLAGS="${RUSTFLAGS:-} -Cprofile-use=${MERGED} -Cllvm-args=-pgo-warn-missing-function -C lto=fat -C codegen-units=1"

echo "[rust-pgo] phase 4: fat LTO rebuild with profile-use (profile=${PROFILE})"
(
  cd "${ROOT}/rust"
  RUSTFLAGS="${USE_FLAGS}" \
    cargo_cmd build --locked --profile "${PROFILE}" --bins
)

echo "[rust-pgo] done: ${MERGED} ($(wc -c <"${MERGED}") bytes)"
