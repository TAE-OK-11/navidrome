package scannerworker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/navidrome/navidrome/core/rustworker"
)

var testBinaryOnce sync.Once

// EnsureTestBinary builds or locates navidrome-scanner for Go tests when
// ND_SCANNERWORKERPATH is unset.
func EnsureTestBinary() error {
	var setupErr error
	testBinaryOnce.Do(func() {
		if configured := strings.TrimSpace(os.Getenv(EnvPath)); configured != "" {
			if _, err := rustworker.ResolveBinary(configured, BinaryName()); err != nil {
				setupErr = err
			}
			return
		}
		root, err := repoRoot()
		if err != nil {
			setupErr = err
			return
		}
		candidate := filepath.Join(root, "rust", "scanner", "target", "release", BinaryName())
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			_ = os.Setenv(EnvPath, candidate)
			return
		}
		cmd := exec.Command("cargo", "+1.98.0", "build", "--release", "--locked")
		cmd.Dir = filepath.Join(root, "rust", "scanner")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			setupErr = fmt.Errorf("building navidrome-scanner for tests: %w", err)
			return
		}
		_ = os.Setenv(EnvPath, candidate)
	})
	return setupErr
}

func repoRoot() (string, error) {
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
