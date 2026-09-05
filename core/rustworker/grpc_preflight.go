package rustworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// GRPCWorkerCheck validates a gRPC-only Rust companion binary at startup.
type GRPCWorkerCheck struct {
	Name     string
	Path     string
	MinBytes int64
	// Health is optional; when set it must succeed before the worker is killed.
	Health func(context.Context, *GRPCProcess) error
}

// PreflightGRPC validates gRPC worker binaries and logs actionable errors.
// It never aborts startup.
func PreflightGRPC(ctx context.Context, checks []GRPCWorkerCheck) {
	for _, check := range checks {
		if err := check.Run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[navidrome] Rust gRPC worker preflight failed for %s (%s): %v\n",
				check.Name, check.Path, err)
		}
	}
}

// PreflightGRPCStrict validates gRPC workers and returns the first failure.
func PreflightGRPCStrict(ctx context.Context, checks []GRPCWorkerCheck) error {
	for _, check := range checks {
		if err := check.Run(ctx); err != nil {
			return fmt.Errorf("%s (%s): %w", check.Name, check.Path, err)
		}
	}
	return nil
}

// Run validates a single gRPC worker binary.
func (c GRPCWorkerCheck) Run(ctx context.Context) error {
	return c.run(ctx)
}

func (c GRPCWorkerCheck) run(ctx context.Context) error {
	if strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("worker path is empty")
	}
	info, err := os.Stat(c.Path)
	if err != nil {
		return fmt.Errorf("worker binary not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("worker path is a directory")
	}
	if c.MinBytes > 0 && info.Size() < c.MinBytes {
		return fmt.Errorf("worker binary is too small (%d bytes); expected a production build", info.Size())
	}
	if c.Health == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, PreflightTimeout)
	defer cancel()

	listen := DefaultListenAddr("preflight-" + c.Name)
	proc, err := StartGRPC(ctx, c.Path, listen, nil)
	if SkipPreflightInTests(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("starting gRPC worker: %w", err)
	}
	defer proc.Close()
	if err := c.Health(ctx, proc); err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	return nil
}

// SkipPreflightInTests reports whether gRPC worker preflight should be skipped.
func SkipPreflightInTests(err error) bool {
	return errors.Is(err, ErrSkippedInTests)
}
