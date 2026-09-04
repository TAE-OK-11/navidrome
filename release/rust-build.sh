#!/bin/sh
# Build Rust gRPC companion binaries with fat LTO (release-fat profile).
#
# Environment:
#   RUST_PROFILE          Cargo profile (default: release-fat)
#   RUSTFLAGS             extra rustc flags (e.g. -C target-cpu=znver3)
#   CARGO_BUILD_JOBS      parallel rustc jobs
set -eu

ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
PROFILE="${RUST_PROFILE:-release-fat}"
JOBS="${CARGO_BUILD_JOBS:-}"

cd "${ROOT}/rust"

cargo_cmd() {
  if command -v rustup >/dev/null 2>&1 && rustup run 1.98.0 cargo --version >/dev/null 2>&1; then
    rustup run 1.98.0 cargo "$@"
  else
    cargo "$@"
  fi
}

DEFAULT_RUSTFLAGS="-C lto=fat -C codegen-units=1"
if [ -n "${RUSTFLAGS:-}" ]; then
  export RUSTFLAGS="${DEFAULT_RUSTFLAGS} ${RUSTFLAGS}"
else
  export RUSTFLAGS="${DEFAULT_RUSTFLAGS}"
fi

ARGS="build --locked --profile ${PROFILE} --bins"
if [ -n "${JOBS}" ]; then
  ARGS="${ARGS} -j ${JOBS}"
fi

echo "[rust] building gRPC workers (profile=${PROFILE}, RUSTFLAGS=${RUSTFLAGS})"
# shellcheck disable=SC2086
CARGO_BUILD_JOBS="${JOBS}" cargo_cmd ${ARGS}
echo "[rust] binaries in ${CARGO_TARGET_DIR:-target}/${PROFILE}/"
