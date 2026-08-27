package scannerworker

import (
	"os"

	"github.com/navidrome/navidrome/core/rustworker"
)

const EnvPath = "ND_SCANNERWORKERPATH"

func BinaryName() string {
	return rustworker.BinaryName("navidrome-scanner")
}

func Resolve() (string, error) {
	return rustworker.ResolveBinary(os.Getenv(EnvPath), BinaryName())
}
