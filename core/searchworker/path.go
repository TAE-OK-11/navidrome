package searchworker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const EnvPath = "ND_SEARCHWORKERPATH"

func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "navidrome-search.exe"
	}
	return "navidrome-search"
}

// Resolve returns the configured search worker, preferring a binary placed
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
