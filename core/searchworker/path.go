package searchworker

import (
	"os"

	"github.com/navidrome/navidrome/core/rustworker"
)

const EnvPath = "ND_SEARCHWORKERPATH"

func BinaryName() string {
	return rustworker.BinaryName("navidrome-search")
}

// Resolve returns the configured search worker, preferring a binary placed
// beside the Navidrome executable before consulting PATH.
func Resolve() (string, error) {
	return rustworker.ResolveBinary(os.Getenv(EnvPath), BinaryName())
}
