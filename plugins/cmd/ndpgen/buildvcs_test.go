package main

import (
	"os"
	"strings"
)

// The integration suite builds ndpgen as a child process. GitHub Actions may
// expose a checkout whose VCS metadata cannot be stamped by that nested build,
// so disable VCS stamping for child Go commands executed by this test binary.
func init() {
	goFlags := strings.TrimSpace(os.Getenv("GOFLAGS"))
	if goFlags != "" {
		goFlags += " "
	}
	if err := os.Setenv("GOFLAGS", goFlags+"-buildvcs=false"); err != nil {
		panic(err)
	}
}
