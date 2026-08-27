package rustworker

import (
	"runtime"
)

// BinaryName returns the platform-specific worker executable name.
func BinaryName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}
