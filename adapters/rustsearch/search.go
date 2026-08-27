// Package rustsearch provides the persistent Rust/Tantivy search companion.
// SQLite FTS remains the fallback while this derived index starts or rebuilds.
package rustsearch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/core/ftsnormalize"
	"github.com/navidrome/navidrome/core/searchworker"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

const (
	protocolVersion        = 1
	MaxResults             = 500
	indexBatchSize         = 1000
	freshnessCheckInterval = 10 * time.Second
	searchRequestTimeout   = 5 * time.Second
	indexRequestTimeout    = 30 * time.Second
	// Prefer a full rebuild when the delta is large enough that chunked
	// upsert/delete commits would spend more time than a single replacement.
	maxIncrementalRatio = 0.25
)

var ErrNotReady = errors.New("Rust search index is not ready")

type document struct {
	Key        string   `json:"key"`
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	LibraryIDs []uint64 `json:"library_ids,omitempty"`
	Primary    string   `json:"primary"`
	Secondary  string   `json:"secondary,omitempty"`
}

type request struct {
	Op         string       `json:"op"`
	Documents  []document   `json:"documents,omitempty"`
	Keys       []string     `json:"keys,omitempty"`
	Query      string       `json:"query,omitempty"`
	LibraryIDs []uint64     `json:"library_ids,omitempty"`
	Searches   []searchSpec `json:"searches,omitempty"`
	Values     []string     `json:"values,omitempty"`
}

type searchSpec struct {
	Kind   string `json:"kind"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type hit struct {
	ID    string  `json:"id"`
	Score float32 `json:"score"`
}

type searchGroup struct {
	Kind string `json:"kind"`
	Hits []hit  `json:"hits"`
}

type response struct {
	Protocol   int           `json:"protocol"`
	OK         bool          `json:"ok"`
	Groups     []searchGroup `json:"groups"`
	Indexed    uint64        `json:"indexed"`
	Error      string        `json:"error"`
	Normalized string        `json:"normalized,omitempty"`
}

type SearchLimits struct {
	Offset int
	Limit  int
}

type SearchResults struct {
	SongIDs   []string
	AlbumIDs  []string
	ArtistIDs []string
}

type worker struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writer  *bufio.Writer
	encoder *json.Encoder
	decoder *json.Decoder
}

type Engine struct {
	gate       chan struct{}
	worker     *worker
	ready      atomic.Bool
	building   atomic.Bool
	generation atomic.Int64
	nextCheck  atomic.Int64
	indexed    atomic.Uint64
}

func Available() bool {
	_, err := searchworker.Resolve()
	return err == nil
}

func New() *Engine {
	e := &Engine{gate: make(chan struct{}, 1)}
	e.gate <- struct{}{}
	return e
}

func (e *Engine) Ready() bool {
	return e != nil && e.ready.Load()
}

func (e *Engine) SearchAll(ctx context.Context, query string, libraryIDs []int, songs, albums, artists SearchLimits) (SearchResults, error) {
	if !e.Ready() {
		return SearchResults{}, ErrNotReady
	}
	resp, err := e.roundTrip(ctx, request{
		Op:         "search_all",
		Query:      query,
		LibraryIDs: libraryScope(libraryIDs),
		Searches: []searchSpec{
			{Kind: "song", Offset: songs.Offset, Limit: songs.Limit},
			{Kind: "album", Offset: albums.Offset, Limit: albums.Limit},
			{Kind: "artist", Offset: artists.Offset, Limit: artists.Limit},
		},
	})
	if err != nil {
		return SearchResults{}, err
	}
	return decodeSearchGroups(resp.Groups)
}

func decodeSearchGroups(groups []searchGroup) (SearchResults, error) {
	var results SearchResults
	seen := 0
	for _, group := range groups {
		ids := make([]string, len(group.Hits))
		for i, hit := range group.Hits {
			ids[i] = hit.ID
		}
		switch group.Kind {
		case "song":
			results.SongIDs = ids
			seen |= 1
		case "album":
			results.AlbumIDs = ids
			seen |= 2
		case "artist":
			results.ArtistIDs = ids
			seen |= 4
		}
	}
	if seen != 7 {
		return SearchResults{}, errors.New("Rust search_all response is missing a result group")
	}
	return results, nil
}

func libraryScope(libraryIDs []int) []uint64 {
	scope := make([]uint64, 0, len(libraryIDs))
	for _, id := range libraryIDs {
		if id > 0 {
			scope = append(scope, uint64(id))
		}
	}
	return scope
}

// RefreshIfStale checks scan generations at a bounded cadence. Searches keep
// using the old index until a replacement or incremental sync commits.
func (e *Engine) RefreshIfStale(ctx context.Context, ds model.DataStore) {
	if e == nil || e.building.Load() {
		return
	}
	now := time.Now()
	next := e.nextCheck.Load()
	if next > now.UnixNano() || !e.nextCheck.CompareAndSwap(next, now.Add(freshnessCheckInterval).UnixNano()) {
		return
	}
	adminCtx := auth.WithAdminUser(context.WithoutCancel(ctx), ds)
	libraries, err := ds.Library(adminCtx).GetAll()
	if err != nil {
		log.Debug(ctx, "Rust search freshness check failed", err)
		return
	}
	ready := e.Ready()
	generation := e.generation.Load()
	if !searchIndexStale(ready, generation, libraries) {
		return
	}
	go func() {
		var err error
		if ready && generation > 0 {
			err = e.RefreshIncremental(adminCtx, ds, generation)
			if err != nil {
				log.Debug(adminCtx, "Rust search incremental refresh failed; falling back to full rebuild", err)
				err = e.Rebuild(adminCtx, ds)
			}
		} else {
			err = e.Rebuild(adminCtx, ds)
		}
		if err != nil {
			log.Warn("Rust search index rebuild failed; SQLite search remains active", err)
		}
	}()
}

// RefreshIncremental applies scan deltas with upsert/delete instead of rebuilding
// the whole in-RAM Tantivy index. Go remains the control tower: it reads SQLite,
// builds documents, and decides when to fall back to Rebuild.
func (e *Engine) RefreshIncremental(ctx context.Context, ds model.DataStore, sinceGeneration int64) error {
	if !e.building.CompareAndSwap(false, true) {
		return nil
	}
	defer e.building.Store(false)
	if !e.ready.Load() || sinceGeneration <= 0 {
		return ErrNotReady
	}

	ctx = auth.WithAdminUser(ctx, ds)
	libraries, err := ds.Library(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("loading libraries for Rust search: %w", err)
	}
	if !searchIndexStale(true, sinceGeneration, libraries) {
		return nil
	}

	since := time.Unix(0, sinceGeneration)
	expected, err := expectedSearchDocuments(ctx, ds)
	if err != nil {
		return err
	}
	indexedBefore := int64(e.indexed.Load())
	if preferFullRebuild(indexedBefore, abs64(expected-indexedBefore)) {
		return e.rebuildLocked(ctx, ds, libraries)
	}

	upserts := make([]document, 0, indexBatchSize)
	deletes := make([]string, 0, indexBatchSize)
	changed := int64(0)
	var lastIndexed uint64
	flushUpserts := func() error {
		if len(upserts) == 0 {
			return nil
		}
		resp, err := e.roundTrip(ctx, request{Op: "upsert", Documents: upserts})
		if err != nil {
			return err
		}
		lastIndexed = resp.Indexed
		upserts = upserts[:0]
		return nil
	}
	flushDeletes := func() error {
		if len(deletes) == 0 {
			return nil
		}
		resp, err := e.roundTrip(ctx, request{Op: "delete", Keys: deletes})
		if err != nil {
			return err
		}
		lastIndexed = resp.Indexed
		deletes = deletes[:0]
		return nil
	}
	queueUpsert := func(doc document) error {
		changed++
		if preferFullRebuild(indexedBefore, changed) {
			return errIncrementalTooLarge
		}
		upserts = append(upserts, doc)
		if len(upserts) >= indexBatchSize {
			return flushUpserts()
		}
		return nil
	}
	queueDelete := func(key string) error {
		changed++
		if preferFullRebuild(indexedBefore, changed) {
			return errIncrementalTooLarge
		}
		deletes = append(deletes, key)
		if len(deletes) >= indexBatchSize {
			return flushDeletes()
		}
		return nil
	}

	if err := e.deltaMediaFiles(ctx, ds, since, queueUpsert, queueDelete); err != nil {
		if errors.Is(err, errIncrementalTooLarge) {
			return e.rebuildLocked(ctx, ds, libraries)
		}
		return err
	}
	if err := e.deltaAlbums(ctx, ds, since, queueUpsert, queueDelete); err != nil {
		if errors.Is(err, errIncrementalTooLarge) {
			return e.rebuildLocked(ctx, ds, libraries)
		}
		return err
	}
	if err := e.deltaArtists(ctx, ds, libraries, since, queueUpsert, queueDelete); err != nil {
		if errors.Is(err, errIncrementalTooLarge) {
			return e.rebuildLocked(ctx, ds, libraries)
		}
		return err
	}
	if err := flushUpserts(); err != nil {
		return err
	}
	if err := flushDeletes(); err != nil {
		return err
	}
	if changed > 0 {
		resp, err := e.roundTrip(ctx, request{Op: "commit"})
		if err != nil {
			return err
		}
		lastIndexed = resp.Indexed
	}
	if changed == 0 {
		if indexedBefore != expected {
			log.Debug(ctx, "Rust search document count drifted without deltas; rebuilding",
				"indexed", indexedBefore, "expected", expected)
			return e.rebuildLocked(ctx, ds, libraries)
		}
		e.generation.Store(scanGeneration(libraries))
		return nil
	}
	if int64(lastIndexed) != expected {
		log.Debug(ctx, "Rust search document count drifted after incremental refresh; rebuilding",
			"indexed", lastIndexed, "expected", expected)
		return e.rebuildLocked(ctx, ds, libraries)
	}

	e.indexed.Store(lastIndexed)
	e.generation.Store(scanGeneration(libraries))
	e.ready.Store(true)
	log.Info(ctx, "Rust search index refreshed incrementally", "documents", lastIndexed, "changed", changed)
	return nil
}

var errIncrementalTooLarge = errors.New("rust search incremental delta is too large")

func preferFullRebuild(indexed, delta int64) bool {
	if indexed <= 0 || delta <= 0 {
		return false
	}
	return float64(delta) > float64(indexed)*maxIncrementalRatio
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func expectedSearchDocuments(ctx context.Context, ds model.DataStore) (int64, error) {
	songs, err := ds.MediaFile(ctx).CountAll(model.QueryOptions{Filters: squirrel.Eq{"missing": false}})
	if err != nil {
		return 0, fmt.Errorf("counting media files for Rust search: %w", err)
	}
	albums, err := ds.Album(ctx).CountAll(model.QueryOptions{Filters: squirrel.Eq{"missing": false}})
	if err != nil {
		return 0, fmt.Errorf("counting albums for Rust search: %w", err)
	}
	artists, err := ds.Artist(ctx).CountAll(model.QueryOptions{Filters: squirrel.Eq{"missing": false}})
	if err != nil {
		return 0, fmt.Errorf("counting artists for Rust search: %w", err)
	}
	return songs + albums + artists, nil
}

func (e *Engine) Rebuild(ctx context.Context, ds model.DataStore) error {
	if !e.building.CompareAndSwap(false, true) {
		return nil
	}
	defer e.building.Store(false)

	ctx = auth.WithAdminUser(ctx, ds)
	libraries, err := ds.Library(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("loading libraries for Rust search: %w", err)
	}
	return e.rebuildLocked(ctx, ds, libraries)
}

func (e *Engine) rebuildLocked(ctx context.Context, ds model.DataStore, libraries model.Libraries) error {
	wasReady := e.ready.Load()
	if _, err := e.roundTrip(ctx, request{Op: "begin_replace"}); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			if _, abortErr := e.roundTrip(cleanupCtx, request{Op: "abort_replace"}); abortErr == nil && wasReady {
				e.ready.Store(true)
			}
		}
	}()

	batch := make([]document, 0, indexBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := e.roundTrip(ctx, request{Op: "append", Documents: batch})
		batch = batch[:0]
		return err
	}
	appendDocument := func(doc document) error {
		batch = append(batch, doc)
		if len(batch) >= indexBatchSize {
			return flush()
		}
		return nil
	}

	if err := e.indexMediaFiles(ctx, ds, appendDocument); err != nil {
		return err
	}
	if err := e.indexAlbums(ctx, ds, appendDocument); err != nil {
		return err
	}
	if err := e.indexArtists(ctx, ds, libraries, appendDocument); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	resp, err := e.roundTrip(ctx, request{Op: "commit_replace"})
	if err != nil {
		return err
	}
	committed = true
	e.indexed.Store(resp.Indexed)
	e.generation.Store(scanGeneration(libraries))
	e.ready.Store(true)
	log.Info(ctx, "Rust search index ready", "documents", resp.Indexed)
	return nil
}

func (e *Engine) indexMediaFiles(ctx context.Context, ds model.DataStore, appendDocument func(document) error) error {
	cursor, err := ds.MediaFile(ctx).GetCursor()
	if err != nil {
		return fmt.Errorf("opening media file cursor for Rust search: %w", err)
	}
	for mediaFile, cursorErr := range cursor {
		if cursorErr != nil {
			return fmt.Errorf("reading media files for Rust search: %w", cursorErr)
		}
		if mediaFile.Missing {
			continue
		}
		if err := appendDocument(e.mediaFileDocument(ctx, mediaFile)); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) deltaMediaFiles(ctx context.Context, ds model.DataStore, since time.Time, upsert func(document) error, deleteKey func(string) error) error {
	cursor, err := ds.MediaFile(ctx).GetCursor(model.QueryOptions{Filters: squirrel.Or{
		squirrel.Gt{"media_file.created_at": since},
		squirrel.Gt{"media_file.updated_at": since},
	}})
	if err != nil {
		return fmt.Errorf("opening media file delta cursor for Rust search: %w", err)
	}
	for mediaFile, cursorErr := range cursor {
		if cursorErr != nil {
			return fmt.Errorf("reading media file deltas for Rust search: %w", cursorErr)
		}
		if mediaFile.Missing {
			if err := deleteKey("song:" + mediaFile.ID); err != nil {
				return err
			}
			continue
		}
		if err := upsert(e.mediaFileDocument(ctx, mediaFile)); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) mediaFileDocument(ctx context.Context, mediaFile model.MediaFile) document {
	secondary := []string{mediaFile.Album, mediaFile.Artist, mediaFile.AlbumArtist,
		mediaFile.SortTitle, mediaFile.SortAlbumName, mediaFile.SortArtistName, mediaFile.SortAlbumArtistName}
	secondary = append(secondary, mediaFile.Participants.AllNames()...)
	ftsValues := append([]string{mediaFile.FullTitle(), mediaFile.Album, mediaFile.Artist, mediaFile.AlbumArtist}, secondary...)
	if norm := e.normalizeFTS(ctx, ftsValues...); norm != "" {
		secondary = append(secondary, norm)
	}
	return document{
		Key: "song:" + mediaFile.ID, ID: mediaFile.ID, Kind: "song",
		LibraryIDs: []uint64{uint64(mediaFile.LibraryID)}, Primary: mediaFile.FullTitle(),
		Secondary: strings.Join(secondary, " "),
	}
}

func (e *Engine) indexAlbums(ctx context.Context, ds model.DataStore, appendDocument func(document) error) error {
	albums, err := ds.Album(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("loading albums for Rust search: %w", err)
	}
	for _, album := range albums {
		if album.Missing {
			continue
		}
		if err := appendDocument(e.albumDocument(ctx, album)); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) deltaAlbums(ctx context.Context, ds model.DataStore, since time.Time, upsert func(document) error, deleteKey func(string) error) error {
	albums, err := ds.Album(ctx).GetAll(model.QueryOptions{Filters: squirrel.Gt{"album.imported_at": since}})
	if err != nil {
		return fmt.Errorf("loading album deltas for Rust search: %w", err)
	}
	for _, album := range albums {
		if album.Missing {
			if err := deleteKey("album:" + album.ID); err != nil {
				return err
			}
			continue
		}
		if err := upsert(e.albumDocument(ctx, album)); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) albumDocument(ctx context.Context, album model.Album) document {
	secondary := []string{album.AlbumArtist, album.SortAlbumName, album.SortAlbumArtistName,
		album.CatalogNum, strings.Join(album.Participants.AllNames(), " ")}
	if norm := e.normalizeFTS(ctx, album.Name, album.AlbumArtist); norm != "" {
		secondary = append(secondary, norm)
	}
	return document{
		Key: "album:" + album.ID, ID: album.ID, Kind: "album",
		LibraryIDs: []uint64{uint64(album.LibraryID)}, Primary: album.FullName(),
		Secondary: strings.Join(secondary, " "),
	}
}

func (e *Engine) indexArtists(ctx context.Context, ds model.DataStore, libraries model.Libraries, appendDocument func(document) error) error {
	return e.collectArtists(ctx, ds, libraries, nil, func(doc document, missing bool) error {
		if missing {
			return nil
		}
		return appendDocument(doc)
	})
}

func (e *Engine) deltaArtists(ctx context.Context, ds model.DataStore, libraries model.Libraries, since time.Time, upsert func(document) error, deleteKey func(string) error) error {
	return e.collectArtists(ctx, ds, libraries, squirrel.Gt{"artist.updated_at": since}, func(doc document, missing bool) error {
		if missing {
			return deleteKey(doc.Key)
		}
		return upsert(doc)
	})
}

func (e *Engine) collectArtists(ctx context.Context, ds model.DataStore, libraries model.Libraries, extraFilter squirrel.Sqlizer, emit func(document, bool) error) error {
	type artistDocument struct {
		artist     model.Artist
		libraryIDs []uint64
	}
	documents := make(map[string]*artistDocument)
	for _, library := range libraries {
		filters := []squirrel.Sqlizer{squirrel.Eq{"library_id": []int{library.ID}}}
		if extraFilter != nil {
			filters = append(filters, extraFilter)
		}
		artists, err := ds.Artist(ctx).GetAll(model.QueryOptions{Filters: squirrel.And(filters)})
		if err != nil {
			return fmt.Errorf("loading artists for Rust search: %w", err)
		}
		for _, artist := range artists {
			indexed := documents[artist.ID]
			if indexed == nil {
				indexed = &artistDocument{artist: artist}
				documents[artist.ID] = indexed
			}
			indexed.libraryIDs = append(indexed.libraryIDs, uint64(library.ID))
		}
	}
	for _, indexed := range documents {
		doc := e.artistDocument(ctx, indexed.artist, indexed.libraryIDs)
		if err := emit(doc, indexed.artist.Missing); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) artistDocument(ctx context.Context, artist model.Artist, libraryIDs []uint64) document {
	secondary := []string{artist.SortArtistName, artist.OrderArtistName}
	if norm := e.normalizeFTS(ctx, artist.Name); norm != "" {
		secondary = append(secondary, norm)
	}
	return document{
		Key: "artist:" + artist.ID, ID: artist.ID, Kind: "artist",
		LibraryIDs: libraryIDs, Primary: artist.Name,
		Secondary: strings.Join(secondary, " "),
	}
}

func (e *Engine) normalizeFTS(ctx context.Context, values ...string) string {
	return ftsnormalize.NormalizeForFTS(ctx, values...)
}

func scanGeneration(libraries model.Libraries) int64 {
	var latest int64
	for _, library := range libraries {
		latest = max(latest, library.LastScanAt.UnixNano())
		latest = max(latest, library.UpdatedAt.UnixNano())
	}
	return latest
}

func searchIndexStale(ready bool, generation int64, libraries model.Libraries) bool {
	return !ready || scanGeneration(libraries) > generation
}

func (e *Engine) roundTrip(ctx context.Context, req request) (response, error) {
	timeout := searchRequestTimeout
	switch req.Op {
	case "begin_replace", "append", "commit_replace", "abort_replace", "upsert", "delete", "commit":
		timeout = indexRequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return response{}, err
	}
	select {
	case <-e.gate:
	case <-ctx.Done():
		return response{}, ctx.Err()
	}
	defer func() { e.gate <- struct{}{} }()

	w, err := e.ensureWorker()
	if err != nil {
		return response{}, err
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		w.kill()
		close(cancelDone)
	})
	finishCancellation := func() {
		if !stopCancel() {
			<-cancelDone
		}
	}
	if err := w.encoder.Encode(req); err != nil {
		finishCancellation()
		e.stopWorker()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return response{}, ctxErr
		}
		return response{}, fmt.Errorf("writing Rust search request: %w", err)
	}
	if err := w.writer.Flush(); err != nil {
		finishCancellation()
		e.stopWorker()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return response{}, ctxErr
		}
		return response{}, fmt.Errorf("flushing Rust search request: %w", err)
	}
	var resp response
	if err := w.decoder.Decode(&resp); err != nil {
		finishCancellation()
		e.stopWorker()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return response{}, ctxErr
		}
		return response{}, fmt.Errorf("reading Rust search response: %w", err)
	}
	finishCancellation()
	if ctxErr := ctx.Err(); ctxErr != nil {
		e.stopWorker()
		return response{}, ctxErr
	}
	if resp.Protocol != protocolVersion {
		e.stopWorker()
		return response{}, fmt.Errorf("unsupported Rust search protocol %d", resp.Protocol)
	}
	if !resp.OK {
		return response{}, fmt.Errorf("Rust search request failed: %s", resp.Error)
	}
	return resp, nil
}

func (e *Engine) ensureWorker() (*worker, error) {
	if e.worker != nil {
		return e.worker, nil
	}
	binary, err := searchworker.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolving Rust search worker: %w", err)
	}
	cmd := exec.Command(binary) //nolint:gosec // administrator-configured or colocated binary
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("starting Rust search worker %q: %w", binary, err)
	}
	bufferedInput := bufio.NewWriterSize(stdin, 256*1024)
	e.worker = &worker{
		cmd: cmd, stdin: stdin, writer: bufferedInput, encoder: json.NewEncoder(bufferedInput),
		decoder: json.NewDecoder(bufio.NewReaderSize(stdout, 256*1024)),
	}
	e.worker.encoder.SetEscapeHTML(false)
	return e.worker, nil
}

func (e *Engine) stopWorker() {
	e.ready.Store(false)
	e.indexed.Store(0)
	if e.worker == nil {
		return
	}
	_ = e.worker.stdin.Close()
	if e.worker.cmd.Process != nil {
		_ = e.worker.cmd.Process.Kill()
	}
	_ = e.worker.cmd.Wait()
	e.worker = nil
}

func (w *worker) kill() {
	if w != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
}
