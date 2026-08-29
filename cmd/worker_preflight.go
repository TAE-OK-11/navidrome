package cmd

import (
	"context"

	"github.com/navidrome/navidrome/core/metadataworker"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker"
	"github.com/navidrome/navidrome/core/searchworker"
)

func preflightRustWorkers(ctx context.Context) {
	checks := make([]rustworker.WorkerCheck, 0, 3)
	if path, err := metadataworker.Resolve(); err == nil {
		checks = append(checks, rustworker.WorkerCheck{
			Name:         "metadata",
			Path:         path,
			Args:         []string{"--normalize-fts-worker"},
			MinBytes:     rustworker.MinMetadataBytes,
			SmokeRequest: rustworker.MetadataNormalizeSmokeRequest(),
			SmokeExpect:  `"normalized":"REM"`,
		})
	}
	if path, err := scannerworker.Resolve(); err == nil {
		checks = append(checks, rustworker.WorkerCheck{
			Name:     "scanner",
			Path:     path,
			MinBytes: rustworker.MinScannerBytes,
		})
	}
	if path, err := searchworker.Resolve(); err == nil {
		checks = append(checks, rustworker.WorkerCheck{
			Name:     "search",
			Path:     path,
			MinBytes: rustworker.MinSearchBytes,
		})
	}
	rustworker.Preflight(ctx, checks)
}
