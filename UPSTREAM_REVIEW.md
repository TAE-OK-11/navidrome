# Upstream compatibility review

Reviewed against official Navidrome `master` at
[`1ceb25c6`](https://github.com/navidrome/navidrome/commit/1ceb25c6c1cc0698efcf164010cf8a25092d0024)
(2026-09-07). This is a selective behavior review; the fork retains its own
workers, transport, caches, persistence changes and UI.

## Origin

The first fork commit, `bbf1a5ffe4b005c5756fad5e69b6b21ff8538758`, has the sole
parent [`7303c9ca`](https://github.com/navidrome/navidrome/commit/7303c9ca474df801d1824a6ca5e799895e38c7db).
`git describe` identifies that parent as `v0.62.0-41-g7303c9ca`: a development
snapshot 41 commits after v0.62.0, dated 2026-06-29. It did not originate from a
v0.63.x release. Later selective ports mean ancestry alone does not identify
which official fixes are missing.

## Selected adaptations

| Official change | Local adaptation |
| --- | --- |
| [`96b051ff`](https://github.com/navidrome/navidrome/commit/96b051ff) partial Native API updates | Preserve omitted radio/library properties and validate the effective library configuration. Keep this fork's caches and scan/watcher lifecycle. Trace through the Go controller wrapper, not just persistence. |
| [`a7365e11`](https://github.com/navidrome/navidrome/commit/a7365e11) smart playlist modification time | Use persisted `updated_at` for `changed`. Combine counter and evaluation timestamps into one SQL update, keeping model/DB timestamps equal and eliminating the extra evaluation write. |
| [`88cd1c39`](https://github.com/navidrome/navidrome/commit/88cd1c39) Deezer quota handling | Recognize errors inside HTTP 200 bodies, retain retryable failures through both artist and this fork's album lookup path, and cap response buffering at 8 MiB. Failed lookups must not become cached “not found” results. |
| [`cb0a6ced`](https://github.com/navidrome/navidrome/commit/cb0a6ced) album tag ordering | Keep first appearance when frequencies tie. Store counts next to ordered values so the comparator does not repeatedly look up three maps. Preserve the existing last-observed text casing. |

Existing local implementations already cover many official optimizations,
including batched playlist path lookup/deletion, participant/genre indexes,
provider retry-delay propagation and AAC MIME handling. Their implementations
were retained rather than replacing them with older/differently scoped code.

Still worthwhile: adapt caller-context public URLs for plugin artwork
(`1f861d27`) across all of this fork's transports, and extend configuration
validation (`bd462840`) to the fork's additional duration options. These require
separate behavior review; neither should be copied wholesale.

## OpenSubsonic coverage

Compared with the [official extension catalog](https://opensubsonic.netlify.app/docs/extensions/),
the server already supports the named music extensions: transcodeOffset,
formPost, songLyrics v1/v2, indexBasedQueue, transcoding, playbackReport,
topSongsByArtistId and apiKeyAuthentication. Sonic similarity is advertised only
when its provider is available. `getPodcastEpisode` is absent along with the
podcast subsystem; it is not advertised without a working implementation.

This review adds the optional
[`ArtistID3.disambiguation`](https://opensubsonic.netlify.app/docs/responses/artistid3/)
field. Artist identity and MusicBrainz IDs do not change. The field passes from
Rust metadata mapping through the existing embedded scan JSON, Go scan batching,
SQLite persistence and artist/index/search responses. No additional RPC or
outbound metadata lookup is introduced. Legacy-client responses omit the
OpenSubsonic fields as before.

The source is explicitly saved `ARTISTCOMMENT` / `ALBUMARTISTCOMMENT` metadata.
These are local tag mappings, not a claim that Picard saves comments by default.
Picard exposes the underlying MusicBrainz descriptions as
[`_artistcomment` / `_albumartistcomment`](https://picard-docs.musicbrainz.org/en/v3.0/variables/variables_basic.html).
A Picard tagging script can save them with:

```text
$set(artistcomment,%_artistcomment%)
$set(albumartistcomment,%_albumartistcomment%)
```

Only unambiguous single-artist credits with one comment are imported. A comment
is never assigned to every artist in a collaboration. Missing tags on another
track do not erase an existing description. To populate existing libraries,
save the tags and rescan changed files (or request a full scan). The schema
migration adds an empty-default column and needs no external data download.
