#!/bin/sh
# Collect representative CPU profiles and merge them into a Go PGO profile.
#
# Run with thin LTO enabled (see release/cgo-lto-env.sh) so profile collection
# stays fast while still exercising CGO-heavy paths.
set -eu

OUTPUT="${PGO_OUTPUT:-default.pgo}"
BENCHTIME="${GO_PGO_BENCHTIME:-3s}"
PROFILE_DIR="${PGO_PROFILE_DIR:-/tmp/pgo}"

if [ -z "${PGO_BUILD_TAGS:-}" ]; then
  if [ -x ./release/build-tags.sh ]; then
    PGO_BUILD_TAGS="$(./release/build-tags.sh)"
  else
    PGO_BUILD_TAGS="netgo,sqlite_fts5"
  fi
fi

mkdir -p "$(dirname "$OUTPUT")"
mkdir -p "${PROFILE_DIR}"

train() {
  name="$1"
  package="$2"
  benchmark="$3"
  benchtime="$4"
  echo "[pgo] training ${name} (${package} ${benchmark})"
  go test \
    -run='^$' \
    -bench="${benchmark}" \
    -benchtime="${benchtime}" \
    -count=1 \
    -tags="${PGO_BUILD_TAGS}" \
    -cpuprofile="${PROFILE_DIR}/${name}.pprof" \
    "${package}"
  test -s "${PROFILE_DIR}/${name}.pprof"
}

train compression ./server '^BenchmarkCompressionLargeSingleWrite$' "${BENCHTIME}"
train api ./server/subsonic '^BenchmarkSubsonicJSONMarshal$' "${BENCHTIME}"
train scanner ./scanner '^BenchmarkScan$' "${BENCHTIME}"
train streaming ./core/stream '^BenchmarkLegacyStreamDecision$' 250ms

go tool pprof \
  -proto \
  -output="${OUTPUT}" \
  "${PROFILE_DIR}/compression.pprof" \
  "${PROFILE_DIR}/api.pprof" \
  "${PROFILE_DIR}/scanner.pprof" \
  "${PROFILE_DIR}/streaming.pprof"

test -s "${OUTPUT}"
echo "[pgo] merged profile written to ${OUTPUT} ($(wc -c <"${OUTPUT}") bytes)"
