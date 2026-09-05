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
	// Listen overrides the ephemeral preflight socket. When empty, a unique
	// unix/TCP address under TempDir is chosen.
	Listen string
	// ExtraEnv is appended to the worker process environment (e.g. search index path).
	ExtraEnv []string
	// Health is optional; when set it must succeed before the worker is killed
	// (Run) or returned (RunKeep).
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

// PreflightGRPCStrictKeep validates workers and returns live processes that
// passed Health so callers can Adopt them into ManagedGRPC. On any failure,
// already-started processes are closed and nil is returned.
func PreflightGRPCStrictKeep(ctx context.Context, checks []GRPCWorkerCheck) (map[string]*GRPCProcess, error) {
	kept := make(map[string]*GRPCProcess, len(checks))
	cleanup := func() {
		for name, proc := range kept {
			proc.Close()
			delete(kept, name)
		}
	}
	for _, check := range checks {
		proc, err := check.RunKeep(ctx)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%s (%s): %w", check.Name, check.Path, err)
		}
		if proc != nil {
			kept[check.Name] = proc
		}
	}
	return kept, nil
}

// Run validates a single gRPC worker binary and stops the process afterward.
func (c GRPCWorkerCheck) Run(ctx context.Context) error {
	proc, err := c.RunKeep(ctx)
	if err != nil {
		return err
	}
	if proc != nil {
		proc.Close()
	}
	return nil
}

// RunKeep validates a worker and returns the live process when Health ran
// successfully. Caller must Close or Adopt the process. When Health is nil,
// only the binary size/path checks run and (nil, nil) is returned.
func (c GRPCWorkerCheck) RunKeep(ctx context.Context) (*GRPCProcess, error) {
	if strings.TrimSpace(c.Path) == "" {
		return nil, fmt.Errorf("worker path is empty")
	}
	info, err := os.Stat(c.Path)
	if err != nil {
		return nil, fmt.Errorf("worker binary not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("worker path is a directory")
	}
	if c.MinBytes > 0 && info.Size() < c.MinBytes {
		return nil, fmt.Errorf("worker binary is too small (%d bytes); expected a production build", info.Size())
	}
	if c.Health == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, PreflightTimeout)
	defer cancel()

	listen := c.Listen
	if listen == "" {
		listen = DefaultListenAddr("preflight-" + c.Name)
	}
	proc, err := StartGRPC(ctx, c.Path, listen, c.ExtraEnv)
	if SkipPreflightInTests(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("starting gRPC worker: %w", err)
	}
	if err := c.Health(ctx, proc); err != nil {
		proc.Close()
		return nil, fmt.Errorf("health check: %w", err)
	}
	return proc, nil
}

// SkipPreflightInTests reports whether gRPC worker preflight should be skipped.
func SkipPreflightInTests(err error) bool {
	return errors.Is(err, ErrSkippedInTests)
}
