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
	"github.com/navidrome/navidrome/core/searchworker"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/utils/str"
)

const (
	protocolVersion        = 1
	indexBatchSize         = 1000
	freshnessCheckInterval = 10 * time.Second
	searchRequestTimeout   = 5 * time.Second
	indexRequestTimeout    = 30 * time.Second
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
	Query      string       `json:"query,omitempty"`
	LibraryIDs []uint64     `json:"library_ids,omitempty"`
	Searches   []searchSpec `json:"searches,omitempty"`
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
	Protocol int           `json:"protocol"`
	OK       bool          `json:"ok"`
	Groups   []searchGroup `json:"groups"`
	Indexed  uint64        `json:"indexed"`
	Error    string        `json:"error"`
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
// using the old index until a replacement commits.
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
	if !searchIndexStale(e.Ready(), e.generation.Load(), libraries) {
		return
	}
	go func() {
		if err := e.Rebuild(adminCtx, ds); err != nil {
			log.Warn("Rust search index rebuild failed; SQLite search remains active", err)
		}
	}()
}

func (e *Engine) Rebuild(ctx context.Context, ds model.DataStore) error {
	if !e.building.CompareAndSwap(false, true) {
		return nil
	}
	defer e.building.Store(false)
	wasReady := e.ready.Load()

	ctx = auth.WithAdminUser(ctx, ds)
	libraries, err := ds.Library(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("loading libraries for Rust search: %w", err)
	}
	if _, err = e.roundTrip(ctx, request{Op: "begin_replace"}); err != nil {
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
		batch = make([]document, 0, indexBatchSize)
		return err
	}
	appendDocument := func(doc document) error {
		batch = append(batch, doc)
		if len(batch) == cap(batch) {
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
		secondary := []string{mediaFile.Album, mediaFile.Artist, mediaFile.AlbumArtist,
			mediaFile.SortTitle, mediaFile.SortAlbumName, mediaFile.SortArtistName, mediaFile.SortAlbumArtistName,
			str.NormalizeForFTS(mediaFile.FullTitle(), mediaFile.Album, mediaFile.Artist, mediaFile.AlbumArtist)}
		secondary = append(secondary, mediaFile.Participants.AllNames()...)
		if err := appendDocument(document{
			Key: "song:" + mediaFile.ID, ID: mediaFile.ID, Kind: "song",
			LibraryIDs: []uint64{uint64(mediaFile.LibraryID)}, Primary: mediaFile.FullTitle(),
			Secondary: strings.Join(secondary, " "),
		}); err != nil {
			return err
		}
	}
	return nil
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
		secondary := []string{album.AlbumArtist, album.SortAlbumName, album.SortAlbumArtistName,
			album.CatalogNum, strings.Join(album.Participants.AllNames(), " "),
			str.NormalizeForFTS(album.Name, album.AlbumArtist)}
		if err := appendDocument(document{
			Key: "album:" + album.ID, ID: album.ID, Kind: "album",
			LibraryIDs: []uint64{uint64(album.LibraryID)}, Primary: album.FullName(),
			Secondary: strings.Join(secondary, " "),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) indexArtists(ctx context.Context, ds model.DataStore, libraries model.Libraries, appendDocument func(document) error) error {
	type artistDocument struct {
		artist     model.Artist
		libraryIDs []uint64
	}
	documents := make(map[string]*artistDocument)
	for _, library := range libraries {
		artists, err := ds.Artist(ctx).GetAll(model.QueryOptions{Filters: squirrel.Eq{"library_id": []int{library.ID}}})
		if err != nil {
			return fmt.Errorf("loading artists for Rust search: %w", err)
		}
		for _, artist := range artists {
			if artist.Missing {
				continue
			}
			indexed := documents[artist.ID]
			if indexed == nil {
				indexed = &artistDocument{artist: artist}
				documents[artist.ID] = indexed
			}
			indexed.libraryIDs = append(indexed.libraryIDs, uint64(library.ID))
		}
	}
	for _, indexed := range documents {
		if err := appendDocument(document{
			Key: "artist:" + indexed.artist.ID, ID: indexed.artist.ID, Kind: "artist",
			LibraryIDs: indexed.libraryIDs, Primary: indexed.artist.Name,
			Secondary: strings.Join([]string{indexed.artist.SortArtistName, indexed.artist.OrderArtistName,
				str.NormalizeForFTS(indexed.artist.Name)}, " "),
		}); err != nil {
			return err
		}
	}
	return nil
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
	case "begin_replace", "append", "commit_replace", "abort_replace":
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
