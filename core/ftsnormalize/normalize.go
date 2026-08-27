package ftsnormalize

import (
	"context"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/utils/str"
)

// NormalizeForFTS returns Rust-normalized FTS secondary tokens when the metadata
// worker is available, otherwise falls back to the legacy Go implementation.
func NormalizeForFTS(ctx context.Context, values ...string) string {
	if len(values) == 0 {
		return ""
	}
	if normalized, err := metadataworker.PersistentNormalizeWorkers().Normalize(ctx, values...); err == nil {
		return normalized
	}
	return str.NormalizeForFTS(values...)
}
