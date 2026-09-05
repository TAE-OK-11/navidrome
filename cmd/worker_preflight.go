package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/apikeyworker"
	apikeysgen "github.com/navidrome/navidrome/core/apikeyworker/gen"
	"github.com/navidrome/navidrome/core/integration"
	"github.com/navidrome/navidrome/core/integration/gen"
	"github.com/navidrome/navidrome/core/metadataworker"
	metadatagen "github.com/navidrome/navidrome/core/metadataworker/gen"
	"github.com/navidrome/navidrome/core/rustworker"
	"github.com/navidrome/navidrome/core/scannerworker"
	scannergen "github.com/navidrome/navidrome/core/scannerworker/gen"
	"github.com/navidrome/navidrome/core/searchworker"
	searchgen "github.com/navidrome/navidrome/core/searchworker/gen"
)

var errWorkerHealthNotOK = errors.New("worker health check returned not ok")

func preflightRustWorkers(ctx context.Context) error {
	checks := make([]rustworker.GRPCWorkerCheck, 0, 4)
	if path, err := metadataworker.Resolve(); err == nil {
		checks = append(checks, rustworker.GRPCWorkerCheck{
			Name:     "metadata",
			Path:     path,
			MinBytes: rustworker.MinMetadataBytes,
			Health:   metadataGRPCHealth,
		})
	}
	if path, err := scannerworker.Resolve(); err == nil {
		checks = append(checks, rustworker.GRPCWorkerCheck{
			Name:     "scanner",
			Path:     path,
			MinBytes: rustworker.MinScannerBytes,
			Health:   scannerGRPCHealth,
		})
	}
	if path, err := searchworker.Resolve(); err == nil {
		checks = append(checks, rustworker.GRPCWorkerCheck{
			Name:     "search",
			Path:     path,
			MinBytes: rustworker.MinSearchBytes,
			Health:   searchGRPCHealth,
		})
	}
	if path, err := apikeyworker.Resolve(); err == nil {
		checks = append(checks, rustworker.GRPCWorkerCheck{
			Name:     "apikeys",
			Path:     path,
			MinBytes: rustworker.MinApikeysBytes,
			Health:   apikeysGRPCHealth,
		})
	}
	if len(checks) > 0 {
		if err := rustworker.PreflightGRPCStrict(ctx, checks); err != nil {
			return fmt.Errorf("rust gRPC worker preflight: %w", err)
		}
	}

	if !conf.Server.Integration.Enabled {
		return nil
	}
	path, err := integration.Resolve()
	if err != nil {
		if rustworker.AllowLegacyNDJSON() {
			return nil
		}
		return fmt.Errorf("integration worker binary: %w", err)
	}
	return rustworker.PreflightGRPCStrict(ctx, []rustworker.GRPCWorkerCheck{{
		Name:     "integration",
		Path:     path,
		MinBytes: rustworker.MinIntegrationBytes,
		Health:   integrationGRPCHealth,
	}})
}

func metadataGRPCHealth(ctx context.Context, proc *rustworker.GRPCProcess) error {
	client := metadatagen.NewMetadataClient(proc.Conn)
	resp, err := client.Health(ctx, &metadatagen.HealthRequest{})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errWorkerHealthNotOK
	}
	return nil
}

func scannerGRPCHealth(ctx context.Context, proc *rustworker.GRPCProcess) error {
	client := scannergen.NewFolderHashClient(proc.Conn)
	resp, err := client.Health(ctx, &scannergen.HealthRequest{})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errWorkerHealthNotOK
	}
	return nil
}

func searchGRPCHealth(ctx context.Context, proc *rustworker.GRPCProcess) error {
	client := searchgen.NewSearchClient(proc.Conn)
	resp, err := client.Health(ctx, &searchgen.HealthRequest{})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errWorkerHealthNotOK
	}
	return nil
}

func apikeysGRPCHealth(ctx context.Context, proc *rustworker.GRPCProcess) error {
	client := apikeysgen.NewApiKeysClient(proc.Conn)
	resp, err := client.Health(ctx, &apikeysgen.HealthRequest{})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errWorkerHealthNotOK
	}
	return nil
}

func integrationGRPCHealth(ctx context.Context, proc *rustworker.GRPCProcess) error {
	client := gen.NewOutboundClient(proc.Conn)
	resp, err := client.Health(ctx, &gen.HealthRequest{})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return errIntegrationHealthNotOK
	}
	return nil
}

// warmRustWorkers starts managed gRPC companions after preflight so the first
// request avoids a cold spawn. Preflight still uses a short-lived process for
// strict health; adopting that process into ManagedGRPC is deferred.
func warmRustWorkers() {
	metadataworker.WarmGRPC()
	scannerworker.WarmGRPC()
	searchworker.WarmGRPC()
	apikeyworker.WarmGRPC()
}
