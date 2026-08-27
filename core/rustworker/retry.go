package rustworker

import (
	"context"
	"fmt"
)

const DefaultRestartAttempts = 2

// Run executes fn with up to attempts tries, stopping the slot between failures.
// Context cancellation kills the worker via kill and returns ctx.Err().
func Run(ctx context.Context, attempts int, kill func(), fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		cancelDone := make(chan struct{})
		stopCancel := context.AfterFunc(ctx, func() {
			kill()
			close(cancelDone)
		})
		err := fn()
		if !stopCancel() {
			<-cancelDone
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt+1 < attempts {
			kill()
		}
	}
	return lastErr
}

// FailAfterRestarts wraps the last worker error after restart attempts.
func FailAfterRestarts(workerName string, err error) error {
	return fmt.Errorf("persistent Rust %s worker failed after restart: %w", workerName, err)
}
