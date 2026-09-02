package apikeyworker

import (
	"os"

	"github.com/navidrome/navidrome/core/rustworker"
)

const EnvPath = "ND_APIKEYSWORKERPATH"

func BinaryName() string {
	return rustworker.BinaryName("navidrome-apikeys")
}

func Resolve() (string, error) {
	return rustworker.ResolveBinary(os.Getenv(EnvPath), BinaryName())
}
