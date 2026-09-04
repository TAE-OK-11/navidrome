package rustworker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoRoot walks upward from the current directory until go.mod is found.
func RepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate repository root")
		}
		dir = parent
	}
}

// EnsureTestBinary locates or builds a Rust workspace member binary for Go
// tests. It prefers ND_*WORKERPATH when set, then rust/target/release (workspace
// layout), then the legacy per-crate target directory.
func EnsureTestBinary(envKey, packageName, binaryName string) error {
	if configured := strings.TrimSpace(os.Getenv(envKey)); configured != "" {
		if _, err := ResolveBinary(configured, binaryName); err != nil {
			return err
		}
		return nil
	}

	root, err := RepoRoot()
	if err != nil {
		return err
	}

	candidates := []string{
		filepath.Join(root, "rust", "target", "release", binaryName),
		filepath.Join(root, "rust", packageName, "target", "release", binaryName),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return os.Setenv(envKey, candidate)
		}
	}

	cmd := exec.Command("cargo", "+1.98.0", "build", "--release", "--locked", "-p", packageName)
	cmd.Dir = filepath.Join(root, "rust")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s for tests: %w", binaryName, err)
	}

	workspaceBinary := filepath.Join(root, "rust", "target", "release", binaryName)
	if info, statErr := os.Stat(workspaceBinary); statErr == nil && !info.IsDir() {
		return os.Setenv(envKey, workspaceBinary)
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return os.Setenv(envKey, candidate)
		}
	}
	return fmt.Errorf("built %s but could not locate release binary", binaryName)
}
