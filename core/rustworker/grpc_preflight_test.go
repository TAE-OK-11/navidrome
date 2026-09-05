package rustworker

import (
	"context"
	"errors"
	"testing"
)

func TestSkipPreflightInTests(t *testing.T) {
	if !SkipPreflightInTests(ErrSkippedInTests) {
		t.Fatal("expected ErrSkippedInTests to skip preflight")
	}
	if SkipPreflightInTests(errors.New("other")) {
		t.Fatal("unexpected skip for unrelated error")
	}
}

func TestGRPCWorkerCheckSkipsInTests(t *testing.T) {
	check := GRPCWorkerCheck{
		Name: "test",
		Path: "/bin/true",
		Health: func(context.Context, *GRPCProcess) error {
			return nil
		},
	}
	// StartGRPC is skipped in tests; health-only checks with a valid path should pass.
	if err := check.Run(context.Background()); err != nil {
		t.Fatalf("Run() in tests: %v", err)
	}
}
