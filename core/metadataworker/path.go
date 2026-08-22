package metadataworker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvPath = "ND_METADATAWORKERPATH"

func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "navidrome-metadata.exe"
	}
	return "navidrome-metadata"
}

// Resolve returns the configured metadata worker, preferring a binary placed
// beside the Navidrome executable before consulting PATH.
func Resolve() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(EnvPath)); configured != "" {
		return exec.LookPath(configured)
	}
	name := BinaryName()
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return exec.LookPath(name)
}
