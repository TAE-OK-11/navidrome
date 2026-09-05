#!/bin/sh
# Canonical SQLite amalgamation CFLAGS for CGO builds.
# Keep in sync with Dockerfile.jbs ARG SQLITE_EXTRA_CFLAGS.
#
# Usage:
#   export SQLITE_EXTRA_CFLAGS="$(./release/sqlite-cflags.sh)"
#   eval "$(./release/cgo-lto-env.sh fat)"
#
# These defines are applied to the go-sqlite3 amalgamation. Go build tags
# (sqlite_fts5, sqlite_stat4, …) still gate optional source files; the defines
# here set compile-time defaults for the light/EPYC Zen3 profile.
set -e

printf '%s' \
  '-DSQLITE_ENABLE_FTS5 ' \
  '-DSQLITE_ENABLE_STAT4 ' \
  '-DSQLITE_OMIT_SHARED_CACHE ' \
  '-DSQLITE_DEFAULT_WAL_AUTOCHECKPOINT=1000 ' \
  '-DSQLITE_DEFAULT_WAL_SYNCHRONOUS=1 ' \
  '-DSQLITE_TEMP_STORE=2 ' \
  '-DSQLITE_DEFAULT_MMAP_SIZE=67108864 ' \
  '-DSQLITE_MAX_MMAP_SIZE=268435456 ' \
  '-DSQLITE_DEFAULT_CACHE_SIZE=-16384 ' \
  '-DSQLITE_DEFAULT_MEMSTATUS=0 ' \
  '-DSQLITE_LIKE_DOESNT_MATCH_BLOBS ' \
  '-DSQLITE_DQS=0 ' \
  '-DSQLITE_DEFAULT_FOREIGN_KEYS=1 ' \
  '-DSQLITE_ENABLE_UPDATE_DELETE_LIMIT ' \
  '-DSQLITE_OMIT_DEPRECATED ' \
  '-DSQLITE_USE_URI=1'
