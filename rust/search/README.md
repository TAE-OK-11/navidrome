# Navidrome Rust search worker

This crate is the incremental search engine used to move relevance ranking and
substring search off SQLite's broad `LIKE` fallback. It keeps a Tantivy index
behind an ordered NDJSON protocol so the Go server can use a persistent process
without CGO or per-query process startup.

The index combines exact-name ranking with Unicode 2–3 character n-grams. This
makes short Korean, Japanese, and Chinese names searchable without scanning all
rows, while retaining weighted primary and secondary fields for Latin text.

The protocol supports complete replacement, incremental upserts/deletes, scoped
queries, and stats. Until the Go synchronization layer has completed an initial
snapshot, SQLite FTS5 remains the authoritative fallback.

