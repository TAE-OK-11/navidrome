package ftsnormalize

import (
	"context"
	"strconv"
	"strings"

	"github.com/jellydator/ttlcache/v3"
	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/log"
	"golang.org/x/sync/singleflight"
)

const normalizeCacheEntries = 8192
const normalizeCacheEntryBytes = 4096

// NormalizeForFTS returns normalized FTS secondary tokens via the Rust
// navidrome-metadata worker. The normalization rules live in rust/fts-normalize.
// Results are cached and singleflighted so scanner Put storms reuse one RPC.
func NormalizeForFTS(ctx context.Context, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	key := cacheKey(values)
	if cached, ok := loadCached(key); ok {
		return cached
	}
	v, err, _ := normalizeSF.Do(key, func() (any, error) {
		if cached, ok := loadCached(key); ok {
			return cached, nil
		}
		normalized, err := metadataworker.PersistentNormalizeWorkers().Normalize(ctx, values...)
		if err != nil {
			return "", err
		}
		storeCached(key, normalized)
		return normalized, nil
	})
	if err != nil {
		log.Warn(ctx, "Rust FTS normalize worker failed", err)
		return ""
	}
	return v.(string)
}

// NormalizeMany normalizes many value-groups in one metadata gRPC round-trip
// when possible, filling gaps via NormalizeForFTS (cached) on failure.
func NormalizeMany(ctx context.Context, groups [][]string) []string {
	return normalizeMany(ctx, groups, metadataworker.NormalizeFtsBatch)
}

func normalizeMany(ctx context.Context, groups [][]string, batch func(context.Context, [][]string) ([]string, error)) []string {
	out := make([]string, len(groups))
	if len(groups) == 0 {
		return out
	}

	missingIdx := make([][]int, 0, len(groups))
	positions := make(map[string]int, len(groups))
	missing := make([][]string, 0, len(groups))
	for i, values := range groups {
		if len(values) == 0 {
			continue
		}
		key := cacheKey(values)
		if cached, ok := loadCached(key); ok {
			out[i] = cached
			continue
		}
		if pos, ok := positions[key]; ok {
			missingIdx[pos] = append(missingIdx[pos], i)
			continue
		}
		positions[key] = len(missing)
		missingIdx = append(missingIdx, []int{i})
		missing = append(missing, values)
	}
	if len(missing) == 0 {
		return out
	}

	batched, err := batch(ctx, missing)
	if err == nil && len(batched) == len(missing) {
		for j, indices := range missingIdx {
			for _, idx := range indices {
				out[idx] = batched[j]
			}
			storeCached(cacheKey(missing[j]), batched[j])
		}
		return out
	}
	if err != nil {
		log.Warn(ctx, "Rust FTS normalize batch failed; falling back per item", err)
	}

	for j, indices := range missingIdx {
		if ctx.Err() != nil {
			break
		}
		normalized := NormalizeForFTS(ctx, missing[j]...)
		for _, idx := range indices {
			out[idx] = normalized
		}
	}
	return out
}

func cacheKey(values []string) string {
	// Length framing avoids collisions between embedded NULs and value boundaries.
	var key strings.Builder
	for _, value := range values {
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

var (
	normalizeCache = ttlcache.New[string, string](ttlcache.WithCapacity[string, string](normalizeCacheEntries))
	normalizeSF    singleflight.Group
)

func loadCached(key string) (string, bool) {
	item := normalizeCache.Get(key)
	if item == nil {
		return "", false
	}
	return item.Value(), true
}

func storeCached(key, value string) {
	// Bound retained bytes as well as entry count. Empty values are successful
	// negative results, especially common for ASCII tags, and must be cached.
	if len(key)+len(value) <= normalizeCacheEntryBytes {
		normalizeCache.Set(key, value, ttlcache.NoTTL)
	}
}
