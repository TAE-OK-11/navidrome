#!/bin/sh
# Profile-guided optimization for Rust gRPC workers (integration hot path first).
#
# 1. Build navidrome-integration with -Cprofile-generate
# 2. Run integration criterion benches against the instrumented library paths
# 3. llvm-profdata merge -> merged.profdata
# 4. Rebuild all workspace gRPC workers with -Cprofile-use + release-fat LTO
#
# Environment:
#   RUST_PGO_DIR            profile output directory (default: /tmp/rust-pgo)
#   RUST_PGO_BENCH_TIME     criterion measurement time in seconds (default: 3)
#   RUST_PROFILE            final Cargo profile (default: release-fat)
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
PGO_DIR="${RUST_PGO_DIR:-/tmp/rust-pgo}"
BENCH_TIME="${RUST_PGO_BENCH_TIME:-3}"
PROFILE="${RUST_PROFILE:-release-fat}"
MERGED="${PGO_DIR}/merged.profdata"
TARGET_DIR="${CARGO_TARGET_DIR:-${ROOT}/rust/target}"
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

echo "[rust-pgo] phase 1: instrumented build (profile-generate)"
(
  cd "${ROOT}/rust"
  RUSTFLAGS="${RUSTFLAGS:-} -Cprofile-generate=${PGO_DIR}" \
    cargo_cmd build --locked --release -p navidrome-integration --bins
)

echo "[rust-pgo] phase 2: collecting profiles (criterion ${BENCH_TIME}s)"
(
  cd "${ROOT}/rust/benchmarks/integration"
  RUSTFLAGS="${RUSTFLAGS:-} -Cprofile-generate=${PGO_DIR}" \
    cargo_cmd bench --bench integration_hotpaths -- --measurement-time "${BENCH_TIME}"
)

PROFRAW_COUNT="$(find "${PGO_DIR}" -name '*.profraw' 2>/dev/null | wc -l | tr -d ' ')"
if [ "${PROFRAW_COUNT}" = "0" ]; then
  echo "[rust-pgo] no .profraw files collected in ${PGO_DIR}" >&2
  exit 1
fi

echo "[rust-pgo] phase 3: merge ${PROFRAW_COUNT} profile(s)"
llvm-profdata merge -o "${MERGED}" "${PGO_DIR}"/*.profraw
test -s "${MERGED}"

echo "[rust-pgo] phase 4: fat LTO rebuild with profile-use (profile=${PROFILE})"
(
  cd "${ROOT}/rust"
  RUSTFLAGS="${RUSTFLAGS:-} -Cprofile-use=${MERGED} -Cllvm-args=-pgo-warn-missing-function" \
    cargo_cmd build --locked --profile "${PROFILE}" --bins
)

echo "[rust-pgo] done: ${MERGED} ($(wc -c <"${MERGED}") bytes)"
