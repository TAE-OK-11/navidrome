package ftsnormalize

import (
	"context"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/log"
)

// NormalizeForFTS returns Rust-normalized FTS secondary tokens from the metadata worker.
func NormalizeForFTS(ctx context.Context, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	normalized, err := metadataworker.PersistentNormalizeWorkers().Normalize(ctx, values...)
	if err != nil {
		log.Error(ctx, "Rust FTS normalize worker unavailable", err)
		return ""
	}
	return normalized
}
