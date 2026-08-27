package rustsearch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

func TestRoundTripCancellationDoesNotWaitForBusyWorker(t *testing.T) {
	t.Parallel()

	engine := New()
	<-engine.gate
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := engine.roundTrip(ctx, request{Op: "search_all"})
		result <- err
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("roundTrip() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("roundTrip() did not stop while waiting for worker ownership")
	}
	engine.gate <- struct{}{}
}

func TestScanGenerationUsesLatestLibraryChange(t *testing.T) {
	t.Parallel()

	older := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	libraries := model.Libraries{
		{LastScanAt: older, UpdatedAt: older},
		{LastScanAt: time.Time{}, UpdatedAt: newer},
	}
	if got := scanGeneration(libraries); got != newer.UnixNano() {
		t.Fatalf("scanGeneration() = %d, want %d", got, newer.UnixNano())
	}
}

func TestSearchIndexStaleRestartsAnUnavailableWorker(t *testing.T) {
	t.Parallel()

	generation := time.Unix(200, 0)
	libraries := model.Libraries{{LastScanAt: generation}}
	if searchIndexStale(true, generation.UnixNano(), libraries) {
		t.Fatal("ready search index with current generation is stale")
	}
	if !searchIndexStale(false, generation.UnixNano(), libraries) {
		t.Fatal("unavailable search worker must rebuild even when generation is current")
	}
}

func TestPreferFullRebuild(t *testing.T) {
	t.Parallel()

	if preferFullRebuild(0, 100) {
		t.Fatal("unknown index size should not force a rebuild")
	}
	if preferFullRebuild(1000, 100) {
		t.Fatal("small deltas should stay incremental")
	}
	if !preferFullRebuild(1000, 251) {
		t.Fatal("deltas above 25% should force a full rebuild")
	}
}

func TestMediaFileDocumentKey(t *testing.T) {
	t.Parallel()

	engine := New()
	doc := engine.mediaFileDocument(context.Background(), model.MediaFile{
		ID: "mf-1", LibraryID: 3, Title: "Blue Monday", Album: "Power", Artist: "NO",
	})
	if doc.Key != "song:mf-1" || doc.Kind != "song" || doc.Primary != "Blue Monday" {
		t.Fatalf("media file document = %#v", doc)
	}
	if len(doc.LibraryIDs) != 1 || doc.LibraryIDs[0] != 3 {
		t.Fatalf("library IDs = %#v", doc.LibraryIDs)
	}
}

func TestAlbumDocumentKey(t *testing.T) {
	t.Parallel()

	engine := New()
	doc := engine.albumDocument(context.Background(), model.Album{ID: "al-1", LibraryID: 2, Name: "Power", AlbumArtist: "NO"})
	if doc.Key != "album:al-1" || doc.Kind != "album" || doc.Primary != "Power" {
		t.Fatalf("album document = %#v", doc)
	}
}

func TestDecodeSearchGroups(t *testing.T) {
	t.Parallel()

	results, err := decodeSearchGroups([]searchGroup{
		{Kind: "song", Hits: []hit{{ID: "song-2"}, {ID: "song-1"}}},
		{Kind: "album", Hits: []hit{{ID: "album-1"}}},
		{Kind: "artist", Hits: []hit{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.SongIDs) != 2 || results.SongIDs[0] != "song-2" || results.SongIDs[1] != "song-1" {
		t.Fatalf("song IDs = %#v", results.SongIDs)
	}
	if len(results.AlbumIDs) != 1 || results.AlbumIDs[0] != "album-1" {
		t.Fatalf("album IDs = %#v", results.AlbumIDs)
	}
	if results.ArtistIDs == nil || len(results.ArtistIDs) != 0 {
		t.Fatalf("artist IDs = %#v, want a present empty result", results.ArtistIDs)
	}
	if _, err := decodeSearchGroups([]searchGroup{{Kind: "song"}}); err == nil {
		t.Fatal("missing groups should fail")
	}
}
