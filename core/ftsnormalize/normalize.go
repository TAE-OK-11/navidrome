package ftsnormalize

import (
	"context"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/log"
)

// NormalizeForFTS returns normalized FTS secondary tokens via the Rust
// navidrome-metadata worker. The normalization rules live in rust/fts-normalize.
func NormalizeForFTS(ctx context.Context, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	normalized, err := metadataworker.PersistentNormalizeWorkers().Normalize(ctx, values...)
	if err != nil {
		log.Warn(ctx, "Rust FTS normalize worker failed", err)
		return ""
	}
	return normalized
}
