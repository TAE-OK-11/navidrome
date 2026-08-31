#!/bin/sh
# Emit shell exports for CGO LTO flags.
#
# Usage:
#   eval "$(./release/cgo-lto-env.sh thin)"   # PGO profile collection
#   eval "$(./release/cgo-lto-env.sh fat)"    # final optimized build
#   eval "$(./release/cgo-lto-env.sh off)"    # disable LTO
set -e

mode="${1:-thin}"
cc="${CC:-clang}"

lto_flags=""
lld_flags=""

case "$mode" in
  thin)
    case "$cc" in
      *clang*)
        lto_flags="-flto=thin"
        lld_flags="-fuse-ld=lld"
        ;;
      *)
        lto_flags="-flto=auto"
        ;;
    esac
    ;;
  fat|full)
    case "$cc" in
      *clang*)
        lto_flags="-flto=full"
        lld_flags="-fuse-ld=lld -Wl,-O2 -Wl,--gc-sections"
        ;;
      *)
        lto_flags="-flto -flto-partition=one"
        ;;
    esac
    ;;
  off)
  ;;
  *)
    echo "usage: $0 thin|fat|off" >&2
    exit 1
    ;;
esac

printf 'export CGO_CFLAGS="%s%s"\n' "${CGO_BASE_CFLAGS:-}" "${lto_flags:+ ${lto_flags}}"
printf 'export CGO_CXXFLAGS="%s%s"\n' "${CGO_BASE_CXXFLAGS:-}" "${lto_flags:+ ${lto_flags}}"
printf 'export CGO_LDFLAGS="%s%s%s"\n' "${CGO_BASE_LDFLAGS:-}" "${lto_flags:+ ${lto_flags}}" "${lld_flags:+ ${lld_flags}}"
