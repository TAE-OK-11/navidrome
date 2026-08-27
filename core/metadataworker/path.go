package metadataworker

import (
	"os"

	"github.com/navidrome/navidrome/core/rustworker"
)

const EnvPath = "ND_METADATAWORKERPATH"

func BinaryName() string {
	return rustworker.BinaryName("navidrome-metadata")
}

// Resolve returns the configured metadata worker, preferring a binary placed
// beside the Navidrome executable before consulting PATH.
func Resolve() (string, error) {
	return rustworker.ResolveBinary(os.Getenv(EnvPath), BinaryName())
}
