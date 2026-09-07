// Package lifecycle tracks process-wide closers (refresh queues, gRPC workers,
// event bus) so shutdown can cancel background work instead of leaking it.
package lifecycle

import (
	"slices"
	"sync"
	"testing"
)

// Closer is a shutdown hook. Close must be safe to call more than once.
type Closer interface {
	Close()
}

var (
	mu      sync.Mutex
	closers []*registration
)

// Register records c for CloseAll. Registrations from `go test` are ignored so
// package tests cannot cancel each other's background work.
func Register(c Closer) func() {
	return register(c, testing.Testing())
}

type registration struct{ closer Closer }

// The returned function releases the registry's reference when a resource is
// closed before shutdown (for example, a crashed or replaced worker).
func register(c Closer, skip bool) func() {
	if skip || c == nil {
		return func() {}
	}
	entry := &registration{closer: c}
	mu.Lock()
	closers = append(closers, entry)
	mu.Unlock()
	return func() {
		mu.Lock()
		defer mu.Unlock()
		if i := slices.Index(closers, entry); i >= 0 {
			closers = slices.Delete(closers, i, i+1)
		}
	}
}

// CloseAll closes registered resources in reverse order and clears the list.
func CloseAll() {
	mu.Lock()
	list := closers
	closers = nil
	mu.Unlock()
	for i := len(list) - 1; i >= 0; i-- {
		list[i].closer.Close()
	}
}
