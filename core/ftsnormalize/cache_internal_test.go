package ftsnormalize

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeBatchDeduplicatesAndCachesEmptyResults(t *testing.T) {
	normalizeCache.DeleteAll()
	t.Cleanup(normalizeCache.DeleteAll)
	calls := 0
	batch := func(_ context.Context, groups [][]string) ([]string, error) {
		calls++
		if len(groups) != 1 {
			t.Fatalf("expected one unique group, got %d", len(groups))
		}
		return []string{""}, nil
	}
	groups := [][]string{{"The Beatles"}, {"The Beatles"}}
	for range 2 {
		got := normalizeMany(context.Background(), groups, batch)
		if len(got) != 2 || got[0] != "" || got[1] != "" {
			t.Fatalf("unexpected result: %v", got)
		}
	}
	if calls != 1 {
		t.Fatalf("empty result caused %d RPCs", calls)
	}
}

func TestNormalizationCacheBoundsAndFraming(t *testing.T) {
	normalizeCache.DeleteAll()
	t.Cleanup(normalizeCache.DeleteAll)
	if cacheKey([]string{"a\x00b", "c"}) == cacheKey([]string{"a", "b\x00c"}) {
		t.Fatal("ambiguous group keys")
	}
	oversized := strings.Repeat("x", normalizeCacheEntryBytes+1)
	storeCached(oversized, "")
	if _, ok := loadCached(oversized); ok {
		t.Fatal("oversized entry retained")
	}
	for i := range normalizeCacheEntries + 1 {
		storeCached(cacheKey([]string{strings.Repeat("x", i%100), string(rune(i))}), "")
	}
	if normalizeCache.Len() > normalizeCacheEntries {
		t.Fatal("cache exceeded capacity")
	}
}
