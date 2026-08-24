package rustsearch

import (
	"testing"
	"time"

	"github.com/navidrome/navidrome/model"
)

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
