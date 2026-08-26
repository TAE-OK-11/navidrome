package metadataworker

import (
	"fmt"
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
		if resolved, err := resolveConfiguredBinary(configured); err == nil {
			return resolved, nil
		} else if filepath.IsAbs(configured) {
			return "", err
		}
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

func resolveConfiguredBinary(configured string) (string, error) {
	if filepath.IsAbs(configured) {
		info, err := os.Stat(configured)
		if err != nil {
			return "", fmt.Errorf("metadata worker not found at %q: %w", configured, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("metadata worker path %q is a directory", configured)
		}
		return configured, nil
	}
	return exec.LookPath(configured)
}
