package scannerworker

import (
	"sync"

	"github.com/navidrome/navidrome/core/rustworker"
)

var testBinaryOnce sync.Once

// EnsureTestBinary builds or locates navidrome-scanner for Go tests when
// ND_SCANNERWORKERPATH is unset.
func EnsureTestBinary() error {
	var setupErr error
	testBinaryOnce.Do(func() {
		setupErr = rustworker.EnsureTestBinary(EnvPath, "navidrome-scanner", BinaryName())
	})
	return setupErr
}
