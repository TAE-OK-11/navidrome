package rustworker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ResolveBinary locates a Rust worker executable. When envValue is set to an
// absolute path it must exist; otherwise the value is resolved via PATH. The
// binary beside the Navidrome executable is preferred over PATH.
func ResolveBinary(envValue, binaryName string) (string, error) {
	if configured := strings.TrimSpace(envValue); configured != "" {
		if resolved, err := resolveConfiguredBinary(configured); err == nil {
			return resolved, nil
		} else if filepath.IsAbs(configured) {
			return "", err
		}
	}
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), binaryName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return exec.LookPath(binaryName)
}

func resolveConfiguredBinary(configured string) (string, error) {
	if filepath.IsAbs(configured) {
		cleaned := filepath.Clean(configured)
		info, err := os.Stat(cleaned) //nolint:gosec // administrator-controlled worker path
		if err != nil {
			return "", fmt.Errorf("worker binary not found at %q: %w", cleaned, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("worker path %q is a directory", cleaned)
		}
		return cleaned, nil
	}
	return exec.LookPath(configured)
}
