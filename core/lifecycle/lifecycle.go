// Package lifecycle tracks process-wide closers (refresh queues, gRPC workers,
// event bus) so shutdown can cancel background work instead of leaking it.
package lifecycle

import (
	"sync"
	"testing"
)

// Closer is a shutdown hook. Close must be safe to call more than once.
type Closer interface {
	Close()
}

var (
	mu      sync.Mutex
	closers []Closer
)

// Register records c for CloseAll. Registrations from `go test` are ignored so
// package tests cannot cancel each other's background work.
func Register(c Closer) {
	register(c, testing.Testing())
}

func register(c Closer, skip bool) {
	if skip || c == nil {
		return
	}
	mu.Lock()
	closers = append(closers, c)
	mu.Unlock()
}

// CloseAll closes registered resources in reverse order and clears the list.
func CloseAll() {
	mu.Lock()
	list := closers
	closers = nil
	mu.Unlock()
	for i := len(list) - 1; i >= 0; i-- {
		list[i].Close()
	}
}
