package ftsnormalize

import (
	"context"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/log"
)

// NormalizeForFTS returns normalized FTS secondary tokens.
func NormalizeForFTS(ctx context.Context, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	normalized, err := metadataworker.PersistentNormalizeWorkers().Normalize(ctx, values...)
	if err == nil && normalized != "" {
		return normalized
	}
	if err != nil {
		log.Trace(ctx, "Rust FTS normalize worker unavailable", err)
	}
	return ""
}
