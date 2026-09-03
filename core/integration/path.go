package integration

import (
	"os"

	"github.com/navidrome/navidrome/core/rustworker"
)

const EnvPath = "ND_INTEGRATIONWORKERPATH"

func BinaryName() string {
	return rustworker.BinaryName("navidrome-integration")
}

func Resolve() (string, error) {
	return rustworker.ResolveBinary(os.Getenv(EnvPath), BinaryName())
}
