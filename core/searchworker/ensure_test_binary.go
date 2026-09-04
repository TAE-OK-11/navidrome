package searchworker

import (
	"sync"

	"github.com/navidrome/navidrome/core/rustworker"
)

var testBinaryOnce sync.Once

// EnsureTestBinary builds or locates navidrome-search for Go tests when
// ND_SEARCHWORKERPATH is unset.
func EnsureTestBinary() error {
	var setupErr error
	testBinaryOnce.Do(func() {
		setupErr = rustworker.EnsureTestBinary(EnvPath, "navidrome-search", BinaryName())
	})
	return setupErr
}
